package jobatlas_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	jobatlas "github.com/Russell-Utopia/job-atlas"
)

func TestDiscoveryCompletesWhenEnabledSourceFinishesWithoutResults(t *testing.T) {
	source := newGatedEmptySource("controlled-empty")
	service, err := jobatlas.Open(jobatlas.Config{
		DatabasePath: filepath.Join(t.TempDir(), "job-atlas.db"),
		Sources:      []jobatlas.CompanySource{source},
	})
	if err != nil {
		t.Fatalf("open service: %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})

	runID, err := service.StartDiscovery(context.Background(), []string{"长沙"})
	if err != nil {
		t.Fatalf("start discovery: %v", err)
	}

	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("enabled source did not start")
	}

	running, err := service.GetDiscovery(context.Background(), runID)
	if err != nil {
		t.Fatalf("get running discovery: %v", err)
	}
	assertEmptyDiscovery(t, running, jobatlas.StatusRunning)

	close(source.release)
	completed := waitForStatus(t, service, runID, jobatlas.StatusCompleted)
	assertEmptyDiscovery(t, completed, jobatlas.StatusCompleted)
}

func TestUnfinishedDiscoveryContinuesAfterServiceRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "job-atlas.db")
	completedSource := newRecordingEmptySource("a-completed")
	interruptedSource := newGatedEmptySource("b-interrupted")
	service, err := jobatlas.Open(jobatlas.Config{
		DatabasePath: databasePath,
		Sources:      []jobatlas.CompanySource{completedSource, interruptedSource},
	})
	if err != nil {
		t.Fatalf("open first service: %v", err)
	}

	runID, err := service.StartDiscovery(context.Background(), []string{"福州"})
	if err != nil {
		t.Fatalf("start discovery: %v", err)
	}
	select {
	case <-interruptedSource.started:
	case <-time.After(time.Second):
		t.Fatal("enabled source did not start before interruption")
	}
	select {
	case scope := <-completedSource.scanned:
		if scope.City != "福州" {
			t.Errorf("completed city = %q, want 福州", scope.City)
		}
	default:
		t.Fatal("first source did not finish before interruption")
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close interrupted service: %v", err)
	}

	alreadyCompletedSource := newRecordingEmptySource("a-completed")
	recoveredSource := newRecordingEmptySource("b-interrupted")
	recoveredService, err := jobatlas.Open(jobatlas.Config{
		DatabasePath: databasePath,
		Sources:      []jobatlas.CompanySource{alreadyCompletedSource, recoveredSource},
	})
	if err != nil {
		t.Fatalf("open recovered service: %v", err)
	}
	t.Cleanup(func() {
		if err := recoveredService.Close(); err != nil {
			t.Errorf("close recovered service: %v", err)
		}
	})

	completed := waitForStatus(t, recoveredService, runID, jobatlas.StatusCompleted)
	assertEmptyDiscovery(t, completed, jobatlas.StatusCompleted)

	select {
	case duplicate := <-alreadyCompletedSource.scanned:
		t.Fatalf("completed work was repeated after recovery: %+v", duplicate)
	default:
	}
	select {
	case scope := <-recoveredSource.scanned:
		if scope.City != "福州" {
			t.Errorf("recovered city = %q, want 福州", scope.City)
		}
	default:
		t.Fatal("unfinished city was not resumed")
	}
	select {
	case duplicate := <-recoveredSource.scanned:
		t.Fatalf("city was scanned more than once after recovery: %+v", duplicate)
	default:
	}
}

func TestRecoveryRequiresOriginalUnfinishedSource(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "job-atlas.db")
	source := newGatedEmptySource("required-source")
	service, err := jobatlas.Open(jobatlas.Config{
		DatabasePath: databasePath,
		Sources:      []jobatlas.CompanySource{source},
	})
	if err != nil {
		t.Fatalf("open first service: %v", err)
	}

	if _, err := service.StartDiscovery(context.Background(), []string{"长沙"}); err != nil {
		t.Fatalf("start discovery: %v", err)
	}
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("required source did not start before interruption")
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close interrupted service: %v", err)
	}

	recoveredService, err := jobatlas.Open(jobatlas.Config{DatabasePath: databasePath})
	if err == nil {
		_ = recoveredService.Close()
		t.Fatal("open recovered service without required source succeeded")
	}
}

type gatedEmptySource struct {
	id      jobatlas.SourceID
	started chan struct{}
	release chan struct{}
}

func newGatedEmptySource(id jobatlas.SourceID) *gatedEmptySource {
	return &gatedEmptySource{
		id:      id,
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (s *gatedEmptySource) ID() jobatlas.SourceID {
	return s.id
}

func (s *gatedEmptySource) DiscoverCompanies(
	ctx context.Context,
	_ jobatlas.CityScope,
	_ func(jobatlas.CompanyInfo) error,
) error {
	select {
	case s.started <- struct{}{}:
	default:
	}

	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type recordingEmptySource struct {
	id      jobatlas.SourceID
	scanned chan jobatlas.CityScope
}

func newRecordingEmptySource(id jobatlas.SourceID) *recordingEmptySource {
	return &recordingEmptySource{
		id:      id,
		scanned: make(chan jobatlas.CityScope, 2),
	}
}

func (s *recordingEmptySource) ID() jobatlas.SourceID {
	return s.id
}

func (s *recordingEmptySource) DiscoverCompanies(
	_ context.Context,
	scope jobatlas.CityScope,
	_ func(jobatlas.CompanyInfo) error,
) error {
	s.scanned <- scope
	return nil
}

func waitForStatus(
	t *testing.T,
	service *jobatlas.Service,
	runID jobatlas.RunID,
	want jobatlas.Status,
) jobatlas.Discovery {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		discovery, err := service.GetDiscovery(context.Background(), runID)
		if err != nil {
			t.Fatalf("get discovery: %v", err)
		}
		if discovery.Status == want {
			return discovery
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("discovery did not reach status %q", want)
	return jobatlas.Discovery{}
}

func assertEmptyDiscovery(t *testing.T, discovery jobatlas.Discovery, wantStatus jobatlas.Status) {
	t.Helper()

	if discovery.Status != wantStatus {
		t.Errorf("status = %q, want %q", discovery.Status, wantStatus)
	}
	if discovery.Jobs == nil {
		t.Error("jobs is nil, want an empty list")
	}
	if len(discovery.Jobs) != 0 {
		t.Errorf("jobs = %v, want no jobs", discovery.Jobs)
	}
	if discovery.Error != nil {
		t.Errorf("error = %q, want nil", *discovery.Error)
	}
}
