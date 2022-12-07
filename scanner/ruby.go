package scanner

import "context"

func configureRuby(ctx context.Context, sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Gemfile", "config.ru")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder: "heroku/buildpacks:20",
		Family:  "Ruby",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}
