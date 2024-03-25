package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mrmelon54/random-source/database"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type IndexingService struct {
	db           *database.Queries
	indexStorage string
	runMut       *sync.Mutex
}

type fileProcessJob struct {
	File billy.File
}

func NewIndexingService(db *database.Queries, indexStorage string) *IndexingService {
	return &IndexingService{
		db:           db,
		indexStorage: indexStorage,
		runMut:       &sync.Mutex{},
	}
}

func (s *IndexingService) Run() {
	// prevent concurrent calls
	if !s.runMut.TryLock() {
		return
	}
	defer s.runMut.Unlock()

	for {
		repo, err := s.db.GetNonProcessedRepo(context.Background())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return
			}
			log.Println("Failed to get non-processed repo:", err)
			return
		}
		clone, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
			URL:           repo.Name,
			ReferenceName: plumbing.NewBranchReferenceName(repo.Branch),
			SingleBranch:  true,
		})
		if err != nil {
			log.Println("Failed to clone repository:", err)
			if errors.Is(err, transport.ErrEmptyRemoteRepository) {
				_ = s.db.RemoveRepository(context.Background(), repo.ID)
			}
			continue
		}
		worktree, err := clone.Worktree()
		if err != nil {
			log.Println("Failed to get repository worktree:", err)
			continue
		}

		// setup workers
		wg := &sync.WaitGroup{}
		const fileProcessWorkers = 8
		wg.Add(fileProcessWorkers)
		filesToProcess := make(chan fileProcessJob, 10)
		for range fileProcessWorkers {
			go s.fileProcessWorker(wg, repo.ID, filesToProcess)
		}

		// find files matching rules
		err = util.Walk(worktree.Filesystem, ".", func(path string, info fs.FileInfo, err error) error {
			// skip dot files
			if strings.HasPrefix(info.Name(), ".") {
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}

			// check if extension is allowed
			ext := filepath.Ext(info.Name())
			if len(ext) < 1 {
				return nil
			}
			ext = strings.TrimPrefix(ext, ".")
			if !slices.Contains(codeExts, ext) {
				return nil
			}

			// file is closed later by the worker
			open, err := worktree.Filesystem.Open(path)
			if err != nil {
				return nil
			}
			filesToProcess <- fileProcessJob{open}
			return nil
		})
		if err != nil {
			log.Printf("Failed to walk files in %s\n", repo.Name)

			// close and wait for workers to finish to prevent issues
			close(filesToProcess)
			wg.Wait()
			return
		}

		// wait for workers to finish processing
		close(filesToProcess)
		wg.Wait()

		err = s.db.UpdateIndexedAt(context.Background(), database.UpdateIndexedAtParams{
			IndexedAt: time.Now(),
			ID:        repo.ID,
		})
		if err != nil {
			log.Printf("Failed to update indexed at time in database '%s': %s\n", repo.Name, err)
			return
		}
	}
}

func (s *IndexingService) fileProcessWorker(wg *sync.WaitGroup, repoId int64, filesToProcess <-chan fileProcessJob) {
	defer wg.Done()
	for job := range filesToProcess {
		lineCount := 0
		sc := bufio.NewScanner(job.File)
		sc.Split(bufio.ScanLines)
		for sc.Scan() {
			lineCount++
		}
		_, err := job.File.Seek(0, io.SeekStart)
		if err != nil {
			_ = job.File.Close()
			continue
		}
		indexId, err := s.db.AddIndexedFile(context.Background(), database.AddIndexedFileParams{
			RepositoryID: repoId,
			Path:         job.File.Name(),
			Lines:        int64(lineCount),
		})
		if err != nil {
			_ = job.File.Close()
			continue
		}
		indexFile, err := os.Create(filepath.Join(s.indexStorage, fmt.Sprintf("%d.bin", indexId)))
		if err != nil {
			_ = job.File.Close()
			continue
		}
		_, _ = io.Copy(indexFile, job.File)
		_ = job.File.Close()
		_ = indexFile.Close()
	}
}
