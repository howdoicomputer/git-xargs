package repository

import (
	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v32/github"
)

// git-xargs concept of a repository that encapsulates three different aspects of a 'repo'
//
// Namely:
//
// * The GitHub package's Repository struct (to make remote changes)
// * The git package's Repository struct (to make local changes to the working tree)
//
// Also includes the repository clone directory and target branch name.
//
// In order to differentiate between "other" repositories it's recommended to use
// gxargsrepo as a variable name.
//
type GitXargsRepository struct {
	RepositoryDir    string
	RepositoryRemote *github.Repository
	RepositoryLocal  *git.Repository
	Branch           string
}
