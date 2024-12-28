package service

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"os"
)

func CheckoutSha(gitSource string, commitSha string, user string, password string, destDir string) error {
	repo, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL: gitSource,
		Auth: &http.BasicAuth{
			Username: user,
			Password: password,
		},
	})
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	hash := plumbing.NewHash(commitSha)
	err = w.Checkout(&git.CheckoutOptions{
		Hash:  hash,
		Force: true,
	})

	err = w.Pull(&git.PullOptions{
		RemoteName: "origin",
	})

	return err
}

func CheckoutRef(gitSource string, ref string, user string, password string, destDir string) error {
	repo, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL: gitSource,
		Auth: &http.BasicAuth{
			Username: user,
			Password: password,
		},
	})
	if err != nil {
		return err
	}
	err = repo.Fetch(&git.FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+%s:refs/remotes/origin/mr-merge", ref)),
		},
		Progress: os.Stdout,
		Force:    true,
		Auth:     &http.BasicAuth{Username: user, Password: password},
	})
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewRemoteReferenceName("origin", "mr-merge"),
		Force:  true,
	})
	return err
}
