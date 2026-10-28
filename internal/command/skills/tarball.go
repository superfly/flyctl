package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/superfly/flyctl/internal/buildinfo"
)

const (
	// skillsRepo is the GitHub repository the skills are distributed from.
	skillsRepo = "superfly/skills"
	// skillsRef is the branch installs track by default.
	skillsRef = "main"
	// skillsPathPrefix is the directory inside the repository that holds one
	// subdirectory per skill.
	skillsPathPrefix = "plugins/fly/skills/"

	// Environment overrides, for testing against a branch or a local tarball.
	envRef     = "FLY_SKILLS_REF"
	envTarball = "FLY_SKILLS_TARBALL"

	// maxSkillFileSize bounds a single extracted file so a hostile tarball
	// can't exhaust memory.
	maxSkillFileSize = 8 << 20
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// SkillFile is one file inside a skill directory.
type SkillFile struct {
	// Rel is the path relative to the skill directory, forward-slashed.
	Rel      string
	Contents []byte
}

// SkillFiles maps a skill directory name to its files.
type SkillFiles map[string][]SkillFile

// source describes where to fetch skills from.
type source struct {
	repo    string
	ref     string // branch, tag or SHA; "" means skillsRef
	tarball string // local path or URL; when set, no API lookup is made
}

func sourceFromEnv() source {
	return source{
		repo:    skillsRepo,
		ref:     os.Getenv(envRef),
		tarball: os.Getenv(envTarball),
	}
}

// pinned reports whether the source already names an exact commit.
func (s source) pinned() bool {
	return shaRe.MatchString(s.ref)
}

func (s source) refOrDefault() string {
	if s.ref == "" {
		return skillsRef
	}
	return s.ref
}

func (s source) shaURL() string {
	return fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", s.repo, s.refOrDefault())
}

func (s source) tarballURL(sha string) string {
	if s.tarball != "" {
		return s.tarball
	}
	ref := sha
	if ref == "" {
		ref = "refs/heads/" + s.refOrDefault()
	}

	return fmt.Sprintf("https://github.com/%s/archive/%s.tar.gz", s.repo, ref)
}

var httpClient = &http.Client{Timeout: 2 * time.Minute}

// fetchLatestSHA returns the bare commit SHA the source ref currently points
// at. A local tarball has no upstream, so it returns "".
func fetchLatestSHA(ctx context.Context, src source) (string, error) {
	if src.pinned() {
		return src.ref, nil
	}
	if src.tarball != "" && !isURL(src.tarball) {
		return "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.shaURL(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	req.Header.Set("Accept", "application/vnd.github.sha")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to look up the latest %s revision: %w", src.repo, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to look up the latest %s revision: HTTP %s", src.repo, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(body))
	if !shaRe.MatchString(sha) {
		return "", fmt.Errorf("unexpected revision %q from GitHub", sha)
	}

	return sha, nil
}

// downloadTarball fetches the skills archive. When sha is known the download
// is pinned to it, so the recorded revision always matches the content.
func downloadTarball(ctx context.Context, src source, sha string) ([]byte, error) {
	location := src.tarballURL(sha)
	if !isURL(location) {
		return os.ReadFile(location)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download Fly skills: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download Fly skills: HTTP %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractSkillFiles pulls every regular file under skillsPathPrefix out of a
// gzipped tarball, grouped by the skill directory directly under the prefix.
// GitHub archives wrap everything in a "<repo>-<ref>/" directory, so the
// prefix is matched anywhere in the entry path rather than at the start.
func extractSkillFiles(tarball []byte) (SkillFiles, error) {
	return extractSkillFilesWithPrefix(tarball, skillsPathPrefix)
}

func extractSkillFilesWithPrefix(tarball []byte, prefix string) (SkillFiles, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("skills archive is not gzipped: %w", err)
	}
	defer gz.Close() //nolint:errcheck

	skills := SkillFiles{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read skills archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		name, rel, ok := splitSkillPath(hdr.Name, prefix)
		if !ok {
			continue
		}
		if hdr.Size > maxSkillFileSize {
			return nil, fmt.Errorf("skill file %s/%s is too large (%d bytes)", name, rel, hdr.Size)
		}
		contents, err := io.ReadAll(io.LimitReader(tr, maxSkillFileSize+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s from skills archive: %w", hdr.Name, err)
		}
		skills[name] = append(skills[name], SkillFile{Rel: rel, Contents: contents})
	}

	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found under %s in the downloaded archive", prefix)
	}

	return skills, nil
}

// splitSkillPath turns an archive entry name into (skill name, path relative
// to the skill dir). It rejects entries outside the prefix, directory entries
// and anything that could escape the skill directory.
func splitSkillPath(entry, prefix string) (name, rel string, ok bool) {
	entry = strings.TrimPrefix(entry, "./")
	idx := strings.Index(entry, prefix)
	if idx < 0 {
		return "", "", false
	}
	// The prefix must sit on a path boundary so "notplugins/fly/skills/" is
	// not mistaken for the real one.
	if idx > 0 && entry[idx-1] != '/' {
		return "", "", false
	}
	after := entry[idx+len(prefix):]
	slash := strings.IndexByte(after, '/')
	if slash < 0 {
		return "", "", false
	}
	name, rel = after[:slash], after[slash+1:]
	if name == "" || rel == "" || strings.HasPrefix(name, ".") {
		return "", "", false
	}
	if cleaned := path.Clean(rel); cleaned != rel || strings.HasPrefix(cleaned, "../") || cleaned == ".." || path.IsAbs(cleaned) {
		return "", "", false
	}

	return name, rel, true
}
