package oci

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type DHTokenResponse struct {
	Token string `json:"token"`
}

type ErrorDetail struct {
	Type   string `json:"Type"`
	Class  string `json:"Class"`
	Name   string `json:"Name"`
	Action string `json:"Action"`
}

type ErrorItem struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Detail  []ErrorDetail `json:"detail"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
}

type Manifest struct {
	Digest    string   `json:"digest"`
	MediaType string   `json:"mediaType"`
	Size      int      `json:"size"`
	Platform  Platform `json:"platform"`
}

type Manifests struct {
	Errors    *[]ErrorItem `json:"errors"`
	MediaType string       `json:"mediaType"`
	Config    Manifest     `json:"config"`
	Manifests []Manifest   `json:"manifests"`
}

func (m *Manifests) Error() error {
	raw, err := json.Marshal(m.Errors)
	if err != nil {
		return nil
	}
	return errors.New(string(raw))
}

type ImageConfig struct {
	Config Config `json:"config"`
}

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	Hostname     string                 `json:"Hostname"`
	Domainname   string                 `json:"Domainname"`
	User         string                 `json:"User"`
	AttachStdin  bool                   `json:"AttachStdin"`
	AttachStdout bool                   `json:"AttachStdout"`
	AttachStderr bool                   `json:"AttachStderr"`
	Tty          bool                   `json:"Tty"`
	OpenStdin    bool                   `json:"OpenStdin"`
	StdinOnce    bool                   `json:"StdinOnce"`
	Env          []string               `json:"Env"`
	Cmd          []string               `json:"Cmd"`
	Image        string                 `json:"Image"`
	Volumes      map[string]struct{}    `json:"Volumes"`
	ExposedPorts map[string]interface{} `json:"ExposedPorts"`
	WorkingDir   string                 `json:"WorkingDir"`
	Entrypoint   []string               `json:"Entrypoint"`
	OnBuild      []string               `json:"OnBuild"`
	Labels       map[string]string      `json:"Labels"`
}

func GetImageConfig(image string, auth *Auth) (*Config, error) {

	headers := map[string]string{
		"Accept": strings.Join([]string{
			"application/vnd.docker.distribution.manifest.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ","),
	}

	registry, image := normalizeRegistryAndImage(image)
	tag, image := getTagAndImage(image)

	header, statusCode, err := getRegistryHeaders(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry headers: %w", err)
	}

	if statusCode == http.StatusUnauthorized && registry == "https://registry-1.docker.io" {
		token, err := getDockerHubToken(header, image, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to get dockerhub token: %w", err)
		}
		headers["Authorization"] = "Bearer " + token
	}

	manifestList, err := fetchManifestList(registry, image, tag, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest list: %w", err)
	}

	if manifestList.Errors != nil {
		return nil, fmt.Errorf("failed to fetch manifest list (api_error): %w", manifestList.Error())
	}

	var configDigest, sha256 string
	var manifest *Manifests

	if strings.Contains(manifestList.MediaType, "manifest.v2") {
		configDigest = manifestList.Config.Digest
	} else {
		pickedManifest := getPrioritizedManifest(manifestList.Manifests)
		if pickedManifest != nil {
			sha256 = pickedManifest.Digest
		} else {
			panic("no manifests exists")
		}

		manifest, err = fetchManifestList(registry, image, sha256, headers)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch manifest: %w", err)
		}

		configDigest = manifest.Config.Digest
	}

	config, err := fetchConfigBlob(registry, image, configDigest, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config blob: %w", err)
	}

	return &config.Config, nil

}

func fetchConfigBlob(registry, image, digest string, headers map[string]string) (*ImageConfig, error) {
	var config ImageConfig
	err := makeHttpRequest[ImageConfig](http.MethodGet, fmt.Sprintf("%s/v2/%s/blobs/%s", registry, image, digest), headers, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func getPrioritizedManifest(manifests []Manifest) *Manifest {
	var pickedManifest *Manifest

	for _, manifest := range manifests {
		if manifest.Platform.Os == "linux" {
			pickedManifest = &manifest
		}
	}

	for _, manifest := range manifests {
		if manifest.Platform.Os == "linux" && manifest.Platform.Architecture == "amd64" {
			pickedManifest = &manifest
		}
	}

	if pickedManifest != nil {
		return pickedManifest
	}

	if len(manifests) > 0 {
		return &manifests[0]
	}

	return nil
}

func makeHttpRequest[T any](method, address string, headers map[string]string, response *T) error {
	client := &http.Client{}
	req, err := http.NewRequest(method, address, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bd, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bd, response)
	if err != nil {
		return err
	}

	return nil
}

func fetchManifestList(registry, image, tag string, headers map[string]string) (*Manifests, error) {
	var manifests Manifests
	err := makeHttpRequest[Manifests](http.MethodGet, fmt.Sprintf("%s/v2/%s/manifests/%s", registry, image, tag), headers, &manifests)
	if err != nil {
		return nil, err
	}

	return &manifests, nil
}

func getDockerHubToken(wwwAuthenticateHeader, image string, auth *Auth) (string, error) {
	wwwAuthenticateHeader = strings.ReplaceAll(wwwAuthenticateHeader, "Bearer ", "")
	parts := strings.Split(wwwAuthenticateHeader, ",")
	authParams := make(map[string]string)
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			authParams[kv[0]] = strings.Trim(kv[1], "\"")
		}
	}

	realm := authParams["realm"]
	if auth != nil {
		realm = fmt.Sprintf("https://%s:%s@auth.docker.io/token", auth.Username, auth.Password)
	}

	var td DHTokenResponse
	err := makeHttpRequest[DHTokenResponse](http.MethodGet, fmt.Sprintf("%s?service=%s&scope=%s", realm, authParams["service"], fmt.Sprintf("repository:%s:pull", image)), map[string]string{}, &td)
	if err != nil {
		return "", err
	}

	return td.Token, nil
}

func getRegistryHeaders(registry string) (string, int, error) {
	resp, err := http.Get(registry + "/v2/")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	headers := resp.Header.Get("www-authenticate")
	return headers, resp.StatusCode, nil
}

func getTagAndImage(image string) (string, string) {
	tag := "latest"
	if strings.Contains(image, ":") {
		parts := strings.Split(image, ":")
		image = parts[0]
		if parts[1] != "latest" {
			tag = "sha256:" + parts[1]
		}
	}

	return tag, image
}

func normalizeRegistryAndImage(image string) (string, string) {
	registry := strings.Split(image, "/")[0]
	if registry == image || (!strings.Contains(registry, ".") && registry != "localhost") {
		registry = "docker.io"
	} else {
		image = strings.Join(strings.Split(image, "/")[1:], "/")
	}

	if registry == "docker.io" && !strings.Contains(image, "/") {
		image = "library/" + image
	}

	if registry == "docker.io" {
		registry = "https://registry-1.docker.io"
	} else if matched, _ := regexp.MatchString(`^localhost(:[0-9]+)?$`, registry); !matched {
		registry = "https://" + registry
	}

	return registry, image
}
