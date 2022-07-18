package scanner

func configureRemix(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("remix.config.js")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family: "Remix",
		Port:   8080,
	}

	if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "sqlite")) {
		env["DATABASE_URL"] = "file:/data/sqlite.db"
		s.Files = templates("templates/remix_prisma")
		s.DockerCommand = "start_with_migrations.sh"
		s.DockerEntrypoint = "sh"
		s.Volumes = []Volume{
			{
				Source:      "data",
				Destination: "/data",
			},
		}
		s.Notice = "\nThis launch configuration uses SQLite on a single, dedicated volume. It will not scale beyond a single VM. Look into 'fly postgres' for a more robust production database. \n"
	} else {
		s.Files = templates("templates/remix")
	}

	s.Env = env
	return s, nil
}
