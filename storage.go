package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type State struct {
	Seen          map[string]bool `json:"seen"`
	LastHeartbeat time.Time       `json:"last_heartbeat"`
}

type store interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, state State) error
}

// decodeState decodifica o JSON salvo, aceitando tanto o formato novo
// ({"seen": {...}, "last_heartbeat": "..."}) quanto o formato antigo, de
// antes do heartbeat existir (só o mapa de IDs: {"id": true, ...}).
func decodeState(data []byte) (State, error) {
	var st State
	if err := json.Unmarshal(data, &st); err == nil && st.Seen != nil {
		return st, nil
	}

	var legacy map[string]bool
	if err := json.Unmarshal(data, &legacy); err != nil {
		return State{}, err
	}
	return State{Seen: legacy}, nil
}

// newStore escolhe a implementação de store com base nas variáveis de ambiente.
func newStore(ctx context.Context) (store, error) {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return fileStore{path: seenFile}, nil
	}

	key := os.Getenv("S3_KEY")
	if key == "" {
		key = seenFile
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar config AWS: %w", err)
	}
	return s3Store{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
		key:    key,
	}, nil
}

type fileStore struct {
	path string
}

func (f fileStore) Load(_ context.Context) (State, error) {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return State{Seen: map[string]bool{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	return decodeState(data)
}

func (f fileStore) Save(_ context.Context, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0644)
}

type s3Store struct {
	client *s3.Client
	bucket string
	key    string
}

func (s s3Store) Load(ctx context.Context) (State, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
	})
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return State{Seen: map[string]bool{}}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("erro ao ler %s/%s do S3: %w", s.bucket, s.key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return State{}, fmt.Errorf("erro ao ler corpo do objeto S3: %w", err)
	}
	state, err := decodeState(data)
	if err != nil {
		return State{}, fmt.Errorf("erro ao decodificar histórico do S3: %w", err)
	}
	return state, nil
}

func (s s3Store) Save(ctx context.Context, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("erro ao salvar histórico no S3: %w", err)
	}
	return nil
}
