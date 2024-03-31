package relmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// type testRepo struct {
// 	*gitRepo
// }

// func (r *testRepo) init(t *testing.T) {
// 	_, err := r.runGit("init")
// 	if err != nil {
// 		t.Fatalf("failed to init repo: %s", err)
// 	}
// }

// func (r *testRepo) commit(t *testing.T, msg string) string {
// 	return r.commitTime(t, msg, time.Now())
// }

// func (r *testRepo) commitTime(t *testing.T, msg string, commitTime time.Time) string {
// 	r.gitRepo.env = append(r.gitRepo.env, "GIT_COMMITTER_DATE="+commitTime.Format(time.RFC3339))
// 	defer func() {
// 		r.gitRepo.env = r.gitRepo.env[:len(r.gitRepo.env)-1]
// 	}()

// 	out, err := r.runGit("commit", "--allow-empty", "-m", msg)
// 	if err != nil {
// 		t.Fatalf("failed to commit: %s", err)
// 	}

// 	fmt.Println(out)

// 	// output can be one of:
// 	//   [branch 17c0a58] commit message
// 	//   [branch (root-commit) 17c0a58] commit message

// 	out = strings.Replace(out, "(root-commit) ", "", -1)
// 	out = out[strings.Index(out, " "):strings.Index(out, "]")]
// 	out = strings.TrimSpace(out)
// 	fmt.Println(out)
// 	out, err = r.runGit("rev-parse", out)
// 	if err != nil {
// 		t.Fatalf("failed to get commit sha: %s", err)
// 	}

// 	return out
// }

// func (r *testRepo) tag(t *testing.T, name string) string {
// 	out, err := r.runGit("tag", name)
// 	if err != nil {
// 		t.Fatalf("failed to tag: %s", err)
// 	}
// 	return out
// }

// func (r *testRepo) branch(t *testing.T, name string) string {
// 	out, err := r.runGit("checkout", "-b", name)
// 	if err != nil {
// 		t.Fatalf("failed to create branch: %s", err)
// 	}
// 	return out
// }

// func newTestGitRepo(t *testing.T) *testRepo {
// 	repo := &testRepo{newGitRepo(t.TempDir())}
// 	repo.init(t)
// 	repo.commitTime(t, "initial commit", time.Now().Add(-time.Hour*24).UTC())
// 	return repo
// }

// func TestCommit(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	sha := repo.commit(t, "second commit")

// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	assert.NoError(t, err)
// 	assert.Equal(t, sha, meta.Commit)
// }

// func TestBranch(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	repo.branch(t, "my-branch")

// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)

// 	assert.Equal(t, "my-branch", meta.Branch)
// }

// func TestRef(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	repo.branch(t, "my-branch")

// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)

// 	assert.Equal(t, "refs/heads/my-branch", meta.Ref)
// }

// func TestGitRefFromGitHubEnvVar(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	t.Setenv("GITHUB_REF", "refs/pull/123/merge")
// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.Equal(t, "refs/pull/123/merge", meta.Ref)
// }

// func TestCommitTime(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	commitTime := time.Now().Truncate(time.Second).Add(-time.Hour * 5)
// 	repo.commitTime(t, "second commit", commitTime)
// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.Equal(t, commitTime.UTC(), meta.CommitTime.UTC())
// }

// func TestDirty(t *testing.T) {
// 	repo := newTestGitRepo(t)
// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.False(t, meta.Dirty)

// 	err = os.WriteFile(repo.wd+"/dirty", []byte("dirty"), 0644)
// 	require.NoError(t, err)

// 	meta, err = GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.True(t, meta.Dirty)
// }

// func TestCurrentTagAndVersion(t *testing.T) {
// 	repo := newTestGitRepo(t)

// 	meta, err := GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.Empty(t, meta.Tag)
// 	assert.Empty(t, meta.Version)

// 	expected := "v2023.10.6-stable.1"
// 	expectedVer, err := version.Parse(expected)
// 	require.NoError(t, err)

// 	repo.tag(t, expected)

// 	meta, err = GenerateReleaseMeta(repo.wd, false)
// 	require.NoError(t, err)
// 	assert.Equal(t, expected, meta.Tag)
// 	assert.Equal(t, &expectedVer, meta.Version)
// }

func TestChannelFromRef_CI(t *testing.T) {
	tests := []struct {
		ref             string
		headRef         string
		expectedChannel string
	}{
		{"refs/heads/master", "master", "master"},
		{"refs/heads/main", "main", "main"},
		{"refs/pull/5432/merge", "pr5432-branch", "pr5432"},
		{"refs/pull/543/merge", "pr543-branch", "pr543"},
		{"refs/pull/54/merge", "pr54-branch", "pr54"},
		{"refs/pull/5/merge", "pr5-branch", "pr5"},
		{"refs/heads/my-branch", "my-branch", "my-branch"},
		{"refs/heads/feature/123", "feature/123", "feature/123"},
		{"refs/heads/feat/launch-v2/databases", "feat/launch-v2/databases", "feat/launch-v2/databases"},
		{"refs/heads/fix/prompt-app-create-on-deploy", "fix/prompt-app-create-on-deploy", "fix/prompt-app-create-on-deploy"},
		{"refs/heads/dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8", "dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8", "dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8"},
		{"refs/tags/v0.1.100", "", "stable"},
		{"refs/tags/v2023.10.20-stable.1", "", "stable"},
		{"refs/tags/v2023.10.20-pr1234.1", "", "pr1234"},
		{"refs/tags/v2023.10.20-my-branch.1", "", "my-branch"},
		{"refs/tags/v2023.10.20-master.1", "", "master"},
		{"refs/tags/v2023.10.20-main.1", "", "main"},
	}

	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			t.Setenv("CI", "true")
			t.Setenv("GITHUB_HEAD_REF", test.headRef)
			ch, err := channelFromRef(test.ref)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedChannel, ch)
		})
	}
}
