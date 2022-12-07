package scanner

import (
"context"
"path/filepath"
)

func configureDockerfile(ctx context.Context, sourceDir string) (*SourceInfo, error) {
if !checksPass(sourceDir, fileExists("Dockerfile")) {
return nil, nil
}

s := &SourceInfo{
DockerfilePath: filepath.Join(sourceDir, "Dockerfile"),
Family:         "Dockerfile",
}

return s, nil
}
