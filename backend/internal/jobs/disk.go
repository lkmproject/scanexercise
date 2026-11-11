package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type diskStore struct {
	root string
	mu   sync.Mutex
}

func newDiskStore(root string) *diskStore {
	_ = os.MkdirAll(root, 0o755)
	return &diskStore{root: root}
}

func (d *diskStore) jobDir(jobID string) string {
	return filepath.Join(d.root, jobID)
}

func (d *diskStore) writeMeta(_ context.Context, job *Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := os.MkdirAll(d.jobDir(job.ID), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d.jobDir(job.ID), "meta.json"), payload, 0o644)
}

func (d *diskStore) readMeta(_ context.Context, jobID string) (*Job, error) {
	path := filepath.Join(d.jobDir(jobID), "meta.json")
	d.mu.Lock()
	data, err := os.ReadFile(path)
	d.mu.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (d *diskStore) writeBlob(_ context.Context, jobID, rel string, data []byte) error {
	path := filepath.Join(d.jobDir(jobID), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (d *diskStore) readBlob(_ context.Context, jobID, rel string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(d.jobDir(jobID), filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

func (d *diskStore) remove(_ context.Context, jobID string) error {
	return os.RemoveAll(d.jobDir(jobID))
}

func (d *diskStore) removeBlobs(_ context.Context, jobID string, rels ...string) error {
	for _, rel := range rels {
		_ = os.RemoveAll(filepath.Join(d.jobDir(jobID), filepath.FromSlash(rel)))
	}
	return nil
}

func (d *diskStore) listIDs(_ context.Context) []string {
	entries, err := os.ReadDir(d.root)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids
}
