package main

import (
	"encoding/base64"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

const bodyFileName = "body"

var repos = make(map[string]*GitRepo)

// GitRepo repesents the git repo for a specific Endpoint URL
type GitRepo struct {
	AppKey      string
	EndpointKey string
	URL         string
	Directory   string
	repo        *git.Repository
	worktree    *git.Worktree
	log         *logrus.Entry
}

// GetGitRepo Returns a GitRepo that provides access to a specific git repository.
func GetGitRepo(appKey string, endpointKey string, url string) (*GitRepo, error) {

	encodedURL := base64.StdEncoding.EncodeToString([]byte(url))
	dir := filepath.Join(configuration.GitRoot, appKey, endpointKey, encodedURL)

	gitRepo, ok := repos[dir]
	if ok {
		gitRepo.log.Debug("Found existing GitRepo")
		return gitRepo, nil
	}

	gitRepo = &GitRepo{AppKey: appKey, EndpointKey: endpointKey, URL: url, Directory: dir}

	gitRepo.log = log.WithFields(logrus.Fields{"module": "git", "app": appKey, "endpoint": endpointKey, "url": url})
	gitRepo.log.Debug("Creating new GitRepo")

	r, err := openOrInitRepo(gitRepo.Directory)
	if err != nil {
		gitRepo.log.Errorf("Error opening or intializing repo. %v", err)
		return nil, err
	}
	gitRepo.repo = r

	w, err := gitRepo.repo.Worktree()
	if err != nil {
		gitRepo.log.Errorf("Error accesing git WorkTree. %v", err)
		return nil, err
	}
	gitRepo.worktree = w

	repos[dir] = gitRepo

	return gitRepo, nil
}

func openOrInitRepo(dir string) (*git.Repository, error) {

	r, err := git.PlainOpen(dir)
	if err != nil {
		if err.Error() == "repository not exists" {
			r, err = git.PlainInit(dir, false)
			if err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// UpdateFeed stores the body of a feed result in Git.
func (g *GitRepo) UpdateFeed(body []byte, checkTime time.Time) (string, bool, error) {

	err := ioutil.WriteFile(filepath.Join(g.Directory, bodyFileName), body, 0644)
	if err != nil {
		g.log.Errorf("Error writing file. %v", err)
	}

	s, err := g.worktree.Status()
	if err != nil {
		g.log.Errorf("Error getting status from working tree. %v", err)
		return "", false, err
	}

	if s.IsClean() {
		ref, err := g.repo.Head()
		if err != nil {
			g.log.Errorf("Error getting Head from repo. %v", err)
			return "", false, err
		}
		return ref.Hash().String(), false, nil
	}

	// Add the file if it is new.
	fs := s.File(bodyFileName)
	if fs.Worktree == '?' {
		g.log.Debugf("Adding new file to repo.")
		_, err := g.worktree.Add(bodyFileName)
		if err != nil {
			g.log.Errorf("Error adding new file to repo. %v", err)
			return "", false, err
		}
	}

	g.log.Debugf("Comitting file (new or updated)")
	h, err := g.worktree.Commit(checkTime.Format(time.RFC3339), &git.CommitOptions{All: true,
		Author: &object.Signature{
			Name:  "FeedMonitor",
			Email: "eric.daugherty@possible.com",
			When:  time.Now(),
		},
	})

	return h.String(), true, nil
}
