package scanner

import "github.com/pkg/errors"

func configureBridgetown(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("Gemfile", "bridgetown")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Bridgetown",
		Port:   4000,
		Statics: []Static{
			{
				GuestPath: "/app/output",
				UrlPrefix: "/",
			},
		},
	}

	rubyVersion, err := extractRubyVersion("Gemfile.lock", "Gemfile", ".ruby_version")
	if err != nil {
		return nil, errors.Wrap(err, "failure extracting Ruby version")
	}

	vars := make(map[string]interface{})
	vars["rubyVersion"] = rubyVersion
	s.Files = templatesExecute("templates/bridgetown", vars)

	return s, nil
}
