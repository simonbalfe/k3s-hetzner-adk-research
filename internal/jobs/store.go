package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	streamName    = "research_jobs"
	consumerGroup = "research-workers"
	jobKeyPrefix  = "research_job:"
	jobTTL        = 24 * time.Hour
)

var ErrNotFound = errors.New("job not found")

type Store struct {
	client *redis.Client
}

func Open(ctx context.Context, rawURL string) (*Store, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}

	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) EnsureConsumerGroup(ctx context.Context) error {
	err := s.client.XGroupCreateMkStream(ctx, streamName, consumerGroup, "0").Err()
	if err == nil {
		return nil
	}

	groups, inspectErr := s.client.XInfoGroups(ctx, streamName).Result()
	if inspectErr != nil {
		return fmt.Errorf("create consumer group: %w", err)
	}
	for _, group := range groups {
		if group.Name == consumerGroup {
			return nil
		}
	}
	return fmt.Errorf("create consumer group: %w", err)
}

func (s *Store) Enqueue(ctx context.Context, query string) (Job, error) {
	id, err := newID()
	if err != nil {
		return Job{}, err
	}

	now := time.Now().UTC()
	job := Job{
		ID:        id,
		Query:     query,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.save(ctx, job); err != nil {
		return Job{}, err
	}

	if err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{"job_id": job.ID},
	}).Err(); err != nil {
		_ = s.client.Del(ctx, jobKey(job.ID)).Err()
		return Job{}, fmt.Errorf("publish job: %w", err)
	}

	return job, nil
}

func (s *Store) Find(ctx context.Context, id string) (Job, error) {
	data, err := s.client.Get(ctx, jobKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("read job: %w", err)
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return Job{}, fmt.Errorf("decode job: %w", err)
	}
	return job, nil
}

func (s *Store) Read(ctx context.Context, consumer string, block time.Duration) (Message, error) {
	streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumer,
		Streams:  []string{streamName, ">"},
		Count:    1,
		Block:    block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return Message{}, nil
	}
	if err != nil {
		return Message{}, fmt.Errorf("read job stream: %w", err)
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return Message{}, nil
	}

	entry := streams[0].Messages[0]
	jobID, ok := entry.Values["job_id"].(string)
	if !ok || jobID == "" {
		return Message{}, fmt.Errorf("stream entry %s has no job_id", entry.ID)
	}
	return Message{StreamID: entry.ID, JobID: jobID}, nil
}

func (s *Store) Ack(ctx context.Context, streamID string) error {
	if err := s.client.XAck(ctx, streamName, consumerGroup, streamID).Err(); err != nil {
		return fmt.Errorf("acknowledge job: %w", err)
	}
	return nil
}

func (s *Store) MarkRunning(ctx context.Context, id string) (Job, error) {
	return s.update(ctx, id, func(job *Job) {
		job.Status = StatusRunning
		job.Error = ""
	})
}

func (s *Store) Complete(ctx context.Context, id, result string) error {
	_, err := s.update(ctx, id, func(job *Job) {
		job.Status = StatusSucceeded
		job.Result = result
		job.Error = ""
	})
	return err
}

func (s *Store) Fail(ctx context.Context, id, message string) error {
	_, err := s.update(ctx, id, func(job *Job) {
		job.Status = StatusFailed
		job.Error = message
	})
	return err
}

func (s *Store) update(ctx context.Context, id string, mutate func(*Job)) (Job, error) {
	job, err := s.Find(ctx, id)
	if err != nil {
		return Job{}, err
	}
	mutate(&job)
	job.UpdatedAt = time.Now().UTC()
	if err := s.save(ctx, job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) save(ctx context.Context, job Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode job: %w", err)
	}
	if err := s.client.Set(ctx, jobKey(job.ID), data, jobTTL).Err(); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	return nil
}

func newID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate job ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func jobKey(id string) string {
	return jobKeyPrefix + id
}
