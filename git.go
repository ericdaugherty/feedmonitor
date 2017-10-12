package main

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"gopkg.in/src-d/go-billy.v3/memfs"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

const bodyFileName = "body"

var repos = make(map[string]*GitRepo)
var reposMu = &sync.Mutex{}

// GitRepo repesents the git repo for a specific Endpoint URL
type GitRepo struct {
	AppKey      string
	EndpointKey string
	URL         string
	Directory   string
	repo        *git.Repository
	log         *logrus.Entry
}

// GetGitRepo Returns a GitRepo that provides access to a specific git repository.
func GetGitRepo(appKey string, endpointKey string, url string) (*GitRepo, error) {

	encodedURL := base32.StdEncoding.EncodeToString([]byte(url))
	if len(encodedURL) > 255 {
		hash := sha1.Sum([]byte(encodedURL))
		encodedURL = hex.EncodeToString(hash[:])
	}

	dir := filepath.Join(configuration.GitRoot, appKey, endpointKey, encodedURL)

	reposMu.Lock()
	gitRepo, ok := repos[dir]
	reposMu.Unlock()
	if ok {
		return gitRepo, nil
	}

	gitRepo = &GitRepo{AppKey: appKey, EndpointKey: endpointKey, URL: url, Directory: dir}

	gitRepo.log = log.WithFields(logrus.Fields{"module": "git", "app": appKey, "endpoint": endpointKey, "url": url})
	gitRepo.log.Debug("Opening GitRepo")

	r, err := openOrInitRepo(gitRepo.Directory)
	if err != nil {
		gitRepo.log.Errorf("Error opening or intializing repo. %v", err)
		return nil, err
	}
	gitRepo.repo = r

	reposMu.Lock()
	repos[dir] = gitRepo
	reposMu.Unlock()

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
		return "", false, err
	}

	w, err := g.repo.Worktree()
	if err != nil {
		g.log.Errorf("Error accesing git WorkTree. %v", err)
		return "", false, err
	}

	s, err := w.Status()
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
		_, err := w.Add(bodyFileName)
		if err != nil {
			g.log.Errorf("Error adding new file to repo. %v", err)
			return "", false, err
		}
	}

	h, err := w.Commit(checkTime.Format(time.RFC3339), &git.CommitOptions{All: true,
		Author: &object.Signature{
			Name:  "FeedMonitor",
			Email: "eric.daugherty@possible.com",
			When:  time.Now(),
		},
	})

	return h.String(), true, nil
}

// GetBody returns the contents of the Reponse body for a given Hash.
func (g *GitRepo) GetBody(hash string) ([]byte, error) {

	var r *git.Repository
	fs := memfs.New()
	r, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{URL: g.Directory})
	if err != nil {
		g.log.Errorf("Error cloning repo: %v into memory. %v", g.Directory, err)
		return nil, err
	}

	w, err := r.Worktree()
	if err != nil {
		g.log.Errorf("Error getting working tree from in-memory repo. %v", err)
		return nil, err
	}

	err = w.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(hash)})
	if err != nil {
		g.log.Errorf("Error checking out for hash: %v Error: %v", hash, err)
		return nil, err
	}

	f, err := fs.Open(bodyFileName)
	if err != nil {
		g.log.Errorf("Error opening body for hash %v Error: %v", hash, err)
		return nil, err
	}

	return ioutil.ReadAll(f)
}
