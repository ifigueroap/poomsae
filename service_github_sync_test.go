package poomsae

import (
	"context"
	"sync"
	"time"

	"github.com/ifigueroap/poomsae/internal/githubsync"
)

type serviceGitHubFetcher struct {
	repositoryCalls chan githubsync.Binding
}

func (f *serviceGitHubFetcher) Repository(_ context.Context, host, owner, repo string) (githubsync.Repository, error) {
	f.repositoryCalls <- githubsync.Binding{Host: host, Owner: owner, Repo: repo}
	return githubsync.Repository{NodeID: "R_exampleNode", ID: 12345, FullName: "example-owner/example-repo"}, nil
}

func (*serviceGitHubFetcher) Issues(context.Context, githubsync.Binding, *time.Time) ([]githubsync.Issue, error) {
	return nil, nil
}

func (*serviceGitHubFetcher) Comments(context.Context, githubsync.Binding, int) ([]githubsync.Comment, error) {
	return nil, nil
}

func (*serviceGitHubFetcher) RepositoryComments(context.Context, githubsync.Binding, *time.Time) (map[int][]githubsync.Comment, bool, error) {
	return nil, true, nil
}

func (*serviceGitHubFetcher) ParentData(context.Context, githubsync.Binding, githubsync.ParentRequest) (githubsync.ParentData, error) {
	return githubsync.ParentData{Scan: githubsync.ParentScanUnsupported}, nil
}

type serviceGitHubProgressFetcher struct {
	issuesStarted chan struct{}
	releaseIssues chan struct{}
	startedOnce   sync.Once
	releaseOnce   sync.Once
}

func (*serviceGitHubProgressFetcher) Repository(context.Context, string, string, string) (githubsync.Repository, error) {
	return githubsync.Repository{NodeID: "R_exampleNode", ID: 12345, FullName: "example-owner/example-repo"}, nil
}

func (f *serviceGitHubProgressFetcher) Issues(ctx context.Context, _ githubsync.Binding, _ *time.Time) ([]githubsync.Issue, error) {
	f.startedOnce.Do(func() { close(f.issuesStarted) })
	select {
	case <-f.releaseIssues:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*serviceGitHubProgressFetcher) Comments(context.Context, githubsync.Binding, int) ([]githubsync.Comment, error) {
	return nil, nil
}

func (*serviceGitHubProgressFetcher) RepositoryComments(context.Context, githubsync.Binding, *time.Time) (map[int][]githubsync.Comment, bool, error) {
	return nil, true, nil
}

func (*serviceGitHubProgressFetcher) ParentData(context.Context, githubsync.Binding, githubsync.ParentRequest) (githubsync.ParentData, error) {
	return githubsync.ParentData{Scan: githubsync.ParentScanUnsupported}, nil
}

func (f *serviceGitHubProgressFetcher) release() {
	f.releaseOnce.Do(func() { close(f.releaseIssues) })
}
