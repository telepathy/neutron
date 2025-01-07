package service

import (
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	agentssh "golang.org/x/crypto/ssh"
	"os"
)

func CheckoutSha(gitSource string, commitSha string, privateKey string, destDir string) error {
	pk, err := gitssh.NewPublicKeysFromFile("git", privateKey, "")
	if err != nil {
		return err
	}
	pk.HostKeyCallback = agentssh.InsecureIgnoreHostKey()
	repo, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL:  gitSource,
		Auth: pk,
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
		Auth:       pk,
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	} else {
		return err
	}
}

func CheckoutRef(gitSource string, ref string, privateKey string, destDir string) error {
	pk, err := gitssh.NewPublicKeysFromFile("git", privateKey, "")
	if err != nil {
		return err
	}
	pk.HostKeyCallback = agentssh.InsecureIgnoreHostKey()
	repo, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL:  gitSource,
		Auth: pk,
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
		Auth:     pk,
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
