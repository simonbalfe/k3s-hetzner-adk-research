package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/simonbalfe/k3s-hetzner-adk-research/internal/jobs"
)

type queue interface {
	Read(context.Context, string, time.Duration) (jobs.Message, error)
	Ack(context.Context, string) error
	Find(context.Context, string) (jobs.Job, error)
	MarkRunning(context.Context, string) (jobs.Job, error)
	Complete(context.Context, string, string) error
	Fail(context.Context, string, string) error
}

type researcher interface {
	Research(context.Context, string) (string, error)
}

type Pool struct {
	queue       queue
	researcher  researcher
	concurrency int
	consumer    string
	timeout     time.Duration
	logger      *slog.Logger
}

func NewPool(q queue, r researcher, concurrency int, consumer string, timeout time.Duration, logger *slog.Logger) (*Pool, error) {
	if q == nil || r == nil || logger == nil {
		return nil, errors.New("queue, researcher, and logger are required")
	}
	if concurrency < 1 || concurrency > 100 {
		return nil, errors.New("concurrency must be between 1 and 100")
	}
	if consumer == "" {
		return nil, errors.New("consumer name is required")
	}
	if timeout <= 0 {
		return nil, errors.New("research timeout must be positive")
	}
	return &Pool{
		queue:       q,
		researcher:  r,
		concurrency: concurrency,
		consumer:    consumer,
		timeout:     timeout,
		logger:      logger,
	}, nil
}

func (p *Pool) Run(ctx context.Context) error {
	var wait sync.WaitGroup
	for index := range p.concurrency {
		wait.Add(1)
		go func() {
			defer wait.Done()
			p.consume(ctx, fmt.Sprintf("%s-%d", p.consumer, index))
		}()
	}
	<-ctx.Done()
	wait.Wait()
	return nil
}

func (p *Pool) consume(ctx context.Context, consumer string) {
	for ctx.Err() == nil {
		message, err := p.queue.Read(ctx, consumer, 5*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.logger.Error("read queue", "consumer", consumer, "error", err)
			continue
		}
		if message.StreamID == "" {
			continue
		}
		p.process(ctx, message)
	}
}

func (p *Pool) process(ctx context.Context, message jobs.Message) {
	ctx, finish := context.WithTimeout(context.WithoutCancel(ctx), p.timeout+10*time.Second)
	defer finish()

	job, err := p.queue.Find(ctx, message.JobID)
	if errors.Is(err, jobs.ErrNotFound) {
		p.ack(ctx, message)
		return
	}
	if err != nil {
		p.logger.Error("load job", "job_id", message.JobID, "error", err)
		return
	}
	if job.Status == jobs.StatusSucceeded || job.Status == jobs.StatusFailed {
		p.ack(ctx, message)
		return
	}

	job, err = p.queue.MarkRunning(ctx, job.ID)
	if err != nil {
		p.logger.Error("mark job running", "job_id", job.ID, "error", err)
		return
	}

	researchCtx, cancel := context.WithTimeout(ctx, p.timeout)
	result, researchErr := p.researcher.Research(researchCtx, job.Query)
	cancel()
	if researchErr != nil {
		p.logger.Error("research failed", "job_id", job.ID, "error", researchErr)
		if err := p.queue.Fail(ctx, job.ID, "research failed; retry the request"); err != nil {
			p.logger.Error("record job failure", "job_id", job.ID, "error", err)
			return
		}
		p.ack(ctx, message)
		return
	}

	if err := p.queue.Complete(ctx, job.ID, result); err != nil {
		p.logger.Error("save research result", "job_id", job.ID, "error", err)
		return
	}
	p.ack(ctx, message)
}

func (p *Pool) ack(ctx context.Context, message jobs.Message) {
	if err := p.queue.Ack(ctx, message.StreamID); err != nil {
		p.logger.Error("acknowledge job", "job_id", message.JobID, "error", err)
	}
}
