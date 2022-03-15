package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/v32/github"
	"github.com/gruntwork-io/git-xargs/types"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/logging"

	"golang.org/x/oauth2"
)

// A configuration struct for passing in overrides to the GitHub vlient
type GithubClientConf struct {
	InternalHost               bool
	GithubEnterpriseHost       string
	GithubEnterpriseOauthToken string
}

// The go-github package satisfies this PullRequest service's interface in production
type githubPullRequestService interface {
	Create(ctx context.Context, owner string, name string, pr *github.NewPullRequest) (*github.PullRequest, *github.Response, error)
	List(ctx context.Context, owner string, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
	RequestReviewers(ctx context.Context, owner string, repo string, number int, reviewers github.ReviewersRequest) (*github.PullRequest, *github.Response, error)
}

// The go-github package satisfies this Repositories service's interface in production
type githubRepositoriesService interface {
	Get(ctx context.Context, owner, repo string) (*github.Repository, *github.Response, error)
	ListByOrg(ctx context.Context, org string, opts *github.RepositoryListByOrgOptions) ([]*github.Repository, *github.Response, error)
}

// GithubClient is the data structure that is common between production code and test code. In production code,
// go-github satisfies the PullRequests and Repositories service interfaces, whereas in test the concrete
// implementations for these same services are mocks that return a static slice of pointers to GitHub repositories,
// or a single pointer to a GitHub repository, as appropriate. This allows us to test the workflow of git-xargs
// without actually making API calls to GitHub when running tests
type GithubClient struct {
	PullRequests githubPullRequestService
	Repositories githubRepositoriesService
	Host         string
}

// Set defaults for the client config
func NewClientConfig() *GithubClientConf {
	return &GithubClientConf{
		InternalHost: false,
	}
}

func NewClient(client *github.Client) GithubClient {
	return GithubClient{
		PullRequests: client.PullRequests,
		Repositories: client.Repositories,
	}
}

func newOauthClient(token string) *http.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return oauth2.NewClient(context.Background(), ts)
}

// ConfigureGithubClient creates a GitHub API client using the user-supplied GITHUB_OAUTH_TOKEN and returns the configured GitHub client
func ConfigureGithubClient(config *GithubClientConf) GithubClient {
	logger := logging.GetLogger("git-xargs")

	if config.InternalHost {
		var githubEnterpriseHost string
		if config.GithubEnterpriseHost == "" {
			githubEnterpriseHost = os.Getenv("GITHUB_ENTERPRISE_HOST")
			if githubEnterpriseHost == "" {
				logger.Panic("You passed the --internal flag without setting a GITHUB_ENTERPRISE_HOST environment variable")
			}
		}

		var githubEnterpriseOauthToken string
		if config.GithubEnterpriseOauthToken == "" {
			githubEnterpriseOauthToken = os.Getenv("GITHUB_ENTERPRISE_OAUTH_TOKEN")
			if githubEnterpriseOauthToken == "" {
				logger.Panic("You passed the --intenral flag without setting a GITHUB_ENTERPRISE_OAUTH_TOKEN")
			}
		}

		logger.Debug(githubEnterpriseHost)
		apiBaseURL := fmt.Sprintf("https://%s/api/v3", githubEnterpriseHost)
		apiUploadURL := fmt.Sprintf("%s/upload", apiBaseURL)

		tc := newOauthClient(githubEnterpriseOauthToken)
		_client, _ := github.NewEnterpriseClient(apiBaseURL, apiUploadURL, tc)
		client := NewClient(_client)
		client.Host = githubEnterpriseHost

		return client
	}

	// Ensure user provided a GITHUB_OAUTH_TOKEN
	GithubOauthToken := os.Getenv("GITHUB_OAUTH_TOKEN")

	tc := newOauthClient(GithubOauthToken)

	// Wrap the go-github client in a GithubClient struct, which is common between production and test code
	client := NewClient(github.NewClient(tc))
	client.Host = "github.com"

	return client
}

// EnsureGithubOauthTokenSet is a sanity check that a value is exported for GITHUB_OAUTH_TOKEN
func EnsureGithubOauthTokenSet() error {
	if os.Getenv("GITHUB_OAUTH_TOKEN") == "" {
		return errors.WithStackTrace(types.NoGithubOauthTokenProvidedErr{})
	}
	return nil
}
