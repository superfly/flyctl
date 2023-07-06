package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func configureDotnet(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("*.csproj", "Microsoft.NET.Sdk.Web")) {
		return nil, nil
	}

	csprojName, csprojPath, err := findCSProjFile(sourceDir)
	if err != nil {
		return nil, nil
	}

	dotnetSdkVersion, err := extractDotnetTargetFramework(csprojPath)
	if err != nil {
		return nil, nil
	}

	// we don't support .NET Framework or .NET version below 6.0
	isDotnetFramework := !strings.Contains(dotnetSdkVersion, ".")
	if isDotnetFramework || dotnetSdkVersion < "6.0" {
		if isDotnetFramework {
			fmt.Println("The .NET Framework is not supported.")
		} else {
			fmt.Println("The .NET version found is", dotnetSdkVersion)
		}
		fmt.Println("We only supports projects with .NET version 6.0 or above.")

		return nil, nil
	}

	s := &SourceInfo{
		Family: ".NET",
		Port:   8080,
	}

	vars := make(map[string]interface{})
	vars["dotnetAppName"] = csprojName
	vars["dotnetSdkVersion"] = dotnetSdkVersion
	s.Files = templatesExecute("templates/dotnet", vars)

	return s, nil
}

func findCSProjFile(dir string) (string, string, error) {
	var csprojName, csprojPath string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".csproj" {
			csprojName = strings.TrimSuffix(info.Name(), ".csproj")
			csprojPath = path
			return filepath.SkipDir // Stop walking the directory
		}

		return nil
	})

	return csprojName, csprojPath, err
}

func extractDotnetTargetFramework(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	pattern := `<TargetFramework>(.*?)<\/TargetFramework>`

	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(string(content))

	if len(match) > 1 {
		sdkVersion := strings.TrimPrefix(match[1], "net")
		sdkVersion = strings.TrimPrefix(sdkVersion, "coreapp") // Handle .NET Core
		return sdkVersion, nil
	}

	return "", fmt.Errorf("failed to extract .NET version")
}
