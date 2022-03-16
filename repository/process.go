package repository

import (
	"github.com/google/go-github/v32/github"
	"github.com/gruntwork-io/git-xargs/config"
	"github.com/gruntwork-io/go-commons/logging"
	"github.com/remeh/sizedwaitgroup"
	"github.com/sirupsen/logrus"
)

// ProcessRepos loops through every repo we've selected and use a WaitGroup so that the processing can happen in parallel
func ProcessRepos(gitxargsConfig *config.GitXargsConfig, repos []*github.Repository) ([]*GitXargsRepository, error) {
	logger := logging.GetLogger("git-xargs")
	var gitXargsRepositories []*GitXargsRepository

	// Limit the number of concurrent goroutines using the MaxConcurrentRepos config value
	// MaxConcurrentRepos == 0 will fall back to unlimited (previous default behavior)
	wg := sizedwaitgroup.New(gitxargsConfig.MaxConcurrentRepos)

	for _, repo := range repos {
		wg.Add()
		go func(gitxargsConfig *config.GitXargsConfig, repo *github.Repository) error {
			defer wg.Done()
			// For each repo, run the supplied command against it and, if it succeeds without error,
			// commit the changes, push the local branch to remote and use the GitHub API to open a pr
			processLocalErr, gxargsrepo := ProcessRepo(gitxargsConfig, repo)
			if processLocalErr != nil {
				logger.WithFields(logrus.Fields{
					"Repo name": repo.GetName(), "Error": processLocalErr,
				}).Debug("Error encountered while processing repo")
				return processLocalErr
			}

			processRemoteErr := UpdateRepoRemote(gitxargsConfig, gxargsrepo)
			if processRemoteErr != nil {
				logger.WithFields(logrus.Fields{
					"Repo name": repo.GetName(), "Error": processLocalErr,
				}).Debug("Error encountered while pushing to repo remote.")
				return processRemoteErr
			}

			gitXargsRepositories = append(gitXargsRepositories, gxargsrepo)
			return nil
		}(gitxargsConfig, repo)
	}
	wg.Wait()

	return gitXargsRepositories, nil
}

// 1. Attempt to clone it to the local filesystem. To avoid conflicts, this generates a new directory for each repo FOR EACH run, so heavy use of this tool may inflate your /tmp/ directory size
// 2. Look up the HEAD ref of the repo, and create a new branch from that ref, specific to this tool so that we can safely make our changes in the branch
// 3. Execute the supplied command against the locally cloned repo
// 4. Look up any worktree changes (deleted files, modified files, new and untracked files) and ADD THEM ALL to the git stage
// 5. Commit these changes with the optionally configurable git commit message, or fall back to the default if it was not provided by the user
// 6. Push the branch containing the new commit to the remote origin
// 7. Via the GitHub API, open a pull request of the newly pushed branch against the main branch of the repo
// 8. Track all successfully opened pull requests via the stats tracker so that we can print them out as part of our final
// run report that is displayed in table format to the operator following each run
func ProcessRepo(config *config.GitXargsConfig, repo *github.Repository) (*GitXargsRepository, error) {
	logger := logging.GetLogger("git-xargs")
	logger.Debug("starting to process repo")
	gxargsrepo := &GitXargsRepository{RepositoryRemote: repo}

	// Create a new temporary directory in the default temp directory of the system, but append
	// git-xargs-<repo-name> to it so that it's easier to find when you're looking for it
	repositoryDir, localRepository, cloneErr := cloneLocalRepository(config, repo)
	if cloneErr != nil {
		return gxargsrepo, cloneErr
	}

	// Get HEAD ref from the repo
	ref, headRefErr := getLocalRepoHeadRef(config, localRepository, repo)
	if headRefErr != nil {
		return gxargsrepo, headRefErr
	}

	// Get the worktree for the given local repository, so we can examine any changes made by script operations
	worktree, worktreeErr := getLocalWorkTree(repositoryDir, localRepository, repo)
	if worktreeErr != nil {
		return gxargsrepo, worktreeErr
	}

	// Create a branch in the locally cloned copy of the repo to hold all the changes that may result from script execution
	// Also, attempt to pull the latest from the remote branch if it exists
	branchName, branchErr := checkoutLocalBranch(config, ref, worktree, repo, localRepository)
	if branchErr != nil {
		return gxargsrepo, branchErr
	}

	gxargsrepo.Branch = branchName.String()

	// Run the specified command
	commandErr := executeCommand(config, repositoryDir, repo)
	if commandErr != nil {
		return gxargsrepo, commandErr
	}

	if err := UpdateRepoLocal(config, repositoryDir, worktree, repo, localRepository); err != nil {
		return gxargsrepo, nil
	}

	gxargsrepo.RepositoryDir = repositoryDir
	gxargsrepo.RepositoryLocal = localRepository

	logger.WithFields(logrus.Fields{
		"Repo name": repo.GetName(),
	}).Info("Repository successfully processed")

	return gxargsrepo, nil
}
