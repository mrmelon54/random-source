package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mrmelon54/random-source/database"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type RepoListService struct {
	db       *database.Queries
	username string
	client   *http.Client
	runMut   *sync.Mutex
}

func NewRepoService(db *database.Queries, username string, client *http.Client) *RepoListService {
	return &RepoListService{
		db:       db,
		username: username,
		client:   client,
		runMut:   &sync.Mutex{},
	}
}

// Run is called by the cron library
func (r *RepoListService) Run() {
	// prevent concurrent calls
	if !r.runMut.TryLock() {
		return
	}
	defer r.runMut.Unlock()

	n := 0
	for {
		page, err := r.fetchRepoPage(n)
		if err != nil {
			log.Println("Failed to fetch from GitHub repo API:", err)
			return
		}
		if len(page) == 0 {
			break
		}
		for _, i := range page {
			if !i.Fork {
				// I think we trust GitHub to give us valid times, famous last words
				t, _ := time.Parse(time.RFC3339, i.UpdatedAt)
				err = r.db.AddRepository(context.Background(), database.AddRepositoryParams{
					Name:      i.CloneUrl,
					Branch:    i.DefaultBranch,
					UpdatedAt: t,
				})
				// this worked fine, now continue
				if err == nil {
					continue
				}
				err = r.db.UpdateRepository(context.Background(), database.UpdateRepositoryParams{
					Branch:    i.DefaultBranch,
					UpdatedAt: t,
					Name:      i.CloneUrl,
				})
				if err != nil {
					log.Printf("Failed to add or update repository %s in database: %s\n", i.FullName, err)
					continue
				}
			}
		}
		n++
	}
}

type githubRepo struct {
	FullName      string `json:"full_name"`
	CloneUrl      string `json:"clone_url"`
	Fork          bool   `json:"fork"`
	Language      string `json:"language"`
	UpdatedAt     string `json:"updated_at"`
	DefaultBranch string `json:"default_branch"`
}

func (r *RepoListService) fetchRepoPage(n int) ([]githubRepo, error) {
	var g []githubRepo
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d", r.username, n), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mrmelon54/random-source/1.0.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	a := io.TeeReader(resp.Body, buf)
	err = json.NewDecoder(a).Decode(&g)
	if err != nil {
		fmt.Println("JSON input:", buf.String())
	}
	return g, err
}
