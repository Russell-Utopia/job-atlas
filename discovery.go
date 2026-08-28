package jobatlas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type RunID string

type SourceID string

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
)

type CityScope struct {
	City string
}

type CompanyInfo struct {
	Name        string
	City        string
	EvidenceRef string
}

type Job struct {
	CompanyName string `json:"companyName"`
	Title       string `json:"title"`
	City        string `json:"city"`
	JD          string `json:"jd"`
	ApplyURL    string `json:"applyUrl"`
}

type Discovery struct {
	Status Status  `json:"status"`
	Jobs   []Job   `json:"jobs"`
	Error  *string `json:"error"`
}

type CompanySource interface {
	ID() SourceID
	DiscoverCompanies(context.Context, CityScope, func(CompanyInfo) error) error
}

type Config struct {
	DatabasePath string
	Sources      []CompanySource
}

var (
	ErrInvalidCities                 = errors.New("cities must contain at least one non-empty city")
	ErrRunNotFound                   = errors.New("discovery run not found")
	errCompanyResultRequiresPipeline = errors.New("company result processing is not available in issue #11")
)

type Service struct {
	store     *discoveryStore
	planner   *discoveryPlanner
	ctx       context.Context
	cancel    context.CancelFunc
	wake      chan struct{}
	workers   sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func Open(config Config) (*Service, error) {
	if err := validateSources(config.Sources); err != nil {
		return nil, err
	}

	store, err := openDiscoveryStore(config.DatabasePath)
	if err != nil {
		return nil, err
	}

	planner := newDiscoveryPlanner(store, config.Sources)
	if err := planner.restore(context.Background()); err != nil {
		_ = store.close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		store:   store,
		planner: planner,
		ctx:     ctx,
		cancel:  cancel,
		wake:    make(chan struct{}, 1),
	}
	service.workers.Add(1)
	go service.runWorker()
	service.notifyWork()
	return service, nil
}

func (s *Service) StartDiscovery(ctx context.Context, cities []string) (RunID, error) {
	scopes, err := cityScopes(cities)
	if err != nil {
		return "", err
	}

	runID, err := newRunID()
	if err != nil {
		return "", fmt.Errorf("create run id: %w", err)
	}

	if err := s.planner.start(ctx, runID, scopes); err != nil {
		return "", fmt.Errorf("persist discovery run: %w", err)
	}
	s.notifyWork()

	return runID, nil
}

func (s *Service) GetDiscovery(ctx context.Context, runID RunID) (Discovery, error) {
	return s.store.getDiscovery(ctx, runID)
}

func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.notifyWork()
		s.workers.Wait()
		s.closeErr = s.store.close()
	})
	return s.closeErr
}

func (s *Service) runWorker() {
	defer s.workers.Done()

	for {
		work, found, err := s.store.claimWork(s.ctx)
		if err != nil {
			return
		}
		if !found {
			select {
			case <-s.ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}

		if err := s.planner.discover(s.ctx, work); err != nil {
			if s.ctx.Err() != nil {
				return
			}
			continue
		}
		if err := s.store.completeWork(s.ctx, work); err != nil {
			return
		}
	}
}

func (s *Service) notifyWork() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

type discoveryPlanner struct {
	store     *discoveryStore
	companies *regionalCompanyDiscovery
	sourceIDs []SourceID
}

func newDiscoveryPlanner(store *discoveryStore, sources []CompanySource) *discoveryPlanner {
	sourcesByID := make(map[SourceID]CompanySource, len(sources))
	sourceIDs := make([]SourceID, 0, len(sources))
	for _, source := range sources {
		id := normalizedSourceID(source)
		sourcesByID[id] = source
		sourceIDs = append(sourceIDs, id)
	}

	return &discoveryPlanner{
		store:     store,
		companies: &regionalCompanyDiscovery{sources: sourcesByID},
		sourceIDs: sourceIDs,
	}
}

func (p *discoveryPlanner) restore(ctx context.Context) error {
	unfinishedSourceIDs, err := p.store.unfinishedSourceIDs(ctx)
	if err != nil {
		return fmt.Errorf("read unfinished sources: %w", err)
	}
	for _, id := range unfinishedSourceIDs {
		if _, configured := p.companies.sources[id]; !configured {
			return fmt.Errorf("unfinished discovery requires company source %q", id)
		}
	}
	if err := p.store.restoreInterruptedWork(ctx); err != nil {
		return fmt.Errorf("restore interrupted work: %w", err)
	}
	return nil
}

func (p *discoveryPlanner) start(ctx context.Context, runID RunID, scopes []CityScope) error {
	return p.store.createRun(ctx, runID, scopes, p.sourceIDs)
}

func (p *discoveryPlanner) discover(ctx context.Context, work workItem) error {
	companyDiscovered := false
	err := p.companies.discover(ctx, work.task, func(CompanyInfo) error {
		companyDiscovered = true
		return errCompanyResultRequiresPipeline
	})
	if err != nil {
		return err
	}
	if companyDiscovered {
		return errCompanyResultRequiresPipeline
	}
	return nil
}

type regionalCompanyDiscovery struct {
	sources map[SourceID]CompanySource
}

type companyDiscoveryTask struct {
	sourceID SourceID
	scope    CityScope
}

func (d *regionalCompanyDiscovery) discover(
	ctx context.Context,
	task companyDiscoveryTask,
	emit func(CompanyInfo) error,
) error {
	source, configured := d.sources[task.sourceID]
	if !configured {
		return fmt.Errorf("company source %q is not configured", task.sourceID)
	}
	return source.DiscoverCompanies(ctx, task.scope, emit)
}

func validateSources(sources []CompanySource) error {
	seen := make(map[SourceID]struct{}, len(sources))
	for _, source := range sources {
		if source == nil {
			return errors.New("company source must not be nil")
		}
		id := normalizedSourceID(source)
		if id == "" {
			return errors.New("company source id must not be empty")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate company source id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func normalizedSourceID(source CompanySource) SourceID {
	return SourceID(strings.TrimSpace(string(source.ID())))
}

func cityScopes(cities []string) ([]CityScope, error) {
	seen := make(map[string]struct{}, len(cities))
	scopes := make([]CityScope, 0, len(cities))
	for _, city := range cities {
		city = strings.TrimSpace(city)
		if city == "" {
			return nil, ErrInvalidCities
		}
		if _, exists := seen[city]; exists {
			continue
		}
		seen[city] = struct{}{}
		scopes = append(scopes, CityScope{City: city})
	}
	if len(scopes) == 0 {
		return nil, ErrInvalidCities
	}
	return scopes, nil
}

func newRunID() (RunID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return RunID("run_" + hex.EncodeToString(value[:])), nil
}
