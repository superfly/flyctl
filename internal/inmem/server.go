package inmem

import (
	"context"
	"fmt"
	"sync"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
)

type Server struct {
	mu sync.Mutex

	apps   map[string]*fly.App     // apps by app name
	images map[imageKey]*fly.Image // images by app name & image ref

	machineSeq int                       // machine id generation
	machines   map[string][]*fly.Machine // machines by app name

	buildSeq int               // build id generation
	builds   map[string]*Build // builds by id

	releaseSeq int                 // release id generation
	releases   map[string]*Release // releases by id
}

func NewServer() *Server {
	return &Server{
		apps:     make(map[string]*fly.App),
		machines: make(map[string][]*fly.Machine),
		images:   make(map[imageKey]*fly.Image),
		builds:   make(map[string]*Build),
		releases: make(map[string]*Release),
	}
}

func (s *Server) Client() *Client {
	return NewClient(s)
}

func (s *Server) FlapsClient(appName string) *FlapsClient {
	return NewFlapsClient(s, appName)
}

func (s *Server) CreateApp(app *fly.App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apps[app.Name]; ok {
		panic(fmt.Sprintf("app name already exists: %q", app.Name))
	}
	s.apps[app.Name] = app
}

func (s *Server) CreateBuild(ctx context.Context, appName string) (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.apps[appName]; ok {
		return nil, fmt.Errorf("app not found: %q", appName)
	}

	s.buildSeq++

	build := &Build{
		ID:        fmt.Sprintf("BUILD%d", s.buildSeq),
		AppName:   appName,
		Status:    "started",
		CreatedAt: time.Now(),
	}
	s.builds[build.ID] = build

	return build, nil
}

func (s *Server) FinishBuild(ctx context.Context, id, status string) (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	build, ok := s.builds[id]
	if ok {
		return nil, fmt.Errorf("build not found: %q", id)
	}
	build.Status = status
	build.WallClockTimeMs = time.Since(build.CreatedAt).Milliseconds()

	return build, nil
}

func (s *Server) CreateRelease(ctx context.Context, appID, clientMutationID, image, platformVersion, strategy string) (*Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int
	for _, r := range s.releases {
		if r.AppID == appID {
			n++
		}
	}

	s.releaseSeq++

	release := &Release{
		ID:               fmt.Sprintf("RELEASE%d", s.releaseSeq),
		AppID:            appID,
		ClientMutationID: clientMutationID,
		Image:            image,
		PlatformVersion:  platformVersion,
		Status:           "pending",
		Strategy:         strategy,
		Version:          n + 1,
	}
	s.releases[release.ID] = release

	return release, nil
}

func (s *Server) UpdateRelease(ctx context.Context, id, clientMutationID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	release := s.releases[id]
	if release == nil {
		return fmt.Errorf("release not found: %q", id)
	}

	release.Status = status

	return nil
}

func (s *Server) CreateImage(ctx context.Context, appName, imageRef string, image *fly.Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.apps[appName]; !ok {
		return fmt.Errorf("app not found: %q", appName)
	}

	other := *image
	s.images[imageKey{appName, imageRef}] = &other
	return nil
}

func (s *Server) Launch(ctx context.Context, appName, name, region string, config *fly.MachineConfig) (*fly.Machine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.machineSeq++
	id := s.machineSeq

	machine := &fly.Machine{
		ID:     fmt.Sprintf("%014x", id),
		Name:   name,
		Region: region,
		State:  "started",
		Config: helpers.Clone(config),
	}
	s.machines[appName] = append(s.machines[appName], machine)

	return machine, nil
}

func (s *Server) GetMachine(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, machine := range s.machines[appName] {
		if machine.ID == machineID {
			return helpers.Clone(machine), nil
		}
	}
	return nil, fmt.Errorf("machine not found: %q", machineID)
}

type Build struct {
	ID              string
	AppName         string
	Status          string
	CreatedAt       time.Time
	WallClockTimeMs int64
}

type Release struct {
	ID               string
	AppID            string
	ClientMutationID string
	Definition       any
	Image            string
	PlatformVersion  string
	Status           string
	Strategy         string
	Version          int
}

type imageKey struct {
	appName, imageRef string
}
