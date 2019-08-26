package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	getter "github.com/hashicorp/go-getter"
)

const BuildersRepo = "github.com/superfly/builders"

var ErrUnknownBuilder = errors.New("unknown builder")

func ResolveNamedBuilderURL(name string) (string, error) {
	builders, err := fetchDefaultBuilders()
	if err != nil {
		return "", err
	}

	if url, ok := builders[name]; ok {
		return url, nil
	}

	return "", ErrUnknownBuilder
}

func fetchBuilder(src, cwd string) (string, error) {
	dst, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}
	// go-getter needs a non-existant target, so use a path inside the temp dir
	dst = path.Join(dst, "builder")

	client := &getter.Client{
		Src:  src,
		Dst:  dst,
		Pwd:  cwd,
		Mode: getter.ClientModeAny,
	}

	if err := client.Get(); err != nil {
		fmt.Println(src, dst, cwd)
		return "", err
	}

	return dst, nil
}

type buildersResp struct {
	Tree []struct {
		Path string
		Type string
	}
}

func fetchDefaultBuilders() (map[string]string, error) {
	builders := map[string]string{}

	resp, err := http.Get("https://api.github.com/repos/superfly/builders/git/trees/master")
	if err != nil {
		return builders, err
	}
	defer resp.Body.Close()

	data := buildersResp{}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return builders, err
	}

	for _, t := range data.Tree {
		if t.Type != "tree" {
			continue
		}

		builders[t.Path] = fmt.Sprintf("%s//%s", BuildersRepo, t.Path)
	}

	return builders, nil
}
