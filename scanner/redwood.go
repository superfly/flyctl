package scanner

import "fmt"

func configureRedwood(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("redwood.toml")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:     "RedwoodJS",
		Port:       8910,
		ReleaseCmd: ".fly/release.sh",
	}

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for RedwoodJS app\n")
	} else {
		s.Files = templates("templates/redwood")
	}

	s.Env = map[string]string{
		"PORT": "8910",
		// Telemetry gravely incrases memory usage, and isn't required
		"REDWOOD_DISABLE_TELEMETRY": "1",
	}

	if checksPass(sourceDir+"/api/db", dirContains("*.prisma", "sqlite")) {
		s.Env["MIGRATE_ON_BOOT"] = "true"
		s.Env["DATABASE_URL"] = "file://data/sqlite.db"
		s.Volumes = []Volume{
			{
				Source:      "data",
				Destination: "/data",
			},
		}
		s.Notice = "\nThis deployment will run an SQLite on a single dedicated volume. The app can't scale beyond a single instance. Look into 'fly postgres' for a more robust production database that supports scaling up. \n"
	}

	return s, nil
}
