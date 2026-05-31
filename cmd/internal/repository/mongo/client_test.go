package mongo

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var testContainerInstance testcontainers.Container

func TestMain(m *testing.M) {
	// Выделяем на запуск контейнера отдельный контекст, чтобы он не аффектил глобальный запуск тестов
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 2*time.Minute)

	req := testcontainers.ContainerRequest{
		Image:        "mongo:8.0",
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForLog("Waiting for connections"),
	}

	container, err := testcontainers.GenericContainer(startupCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	startupCancel() // Закрываем контекст сразу после создания контейнера

	if err != nil {
		fmt.Printf("Failed to start container: %v\n", err)
		os.Exit(1)
	}

	testContainerInstance = container

	// Запуск всех тестов пакета
	code := m.Run()

	// Очистка контейнера после выполнения тестов
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = testContainerInstance.Terminate(shutdownCtx)

	os.Exit(code)
}

func setupTestDB(t *testing.T) (*Repository, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	host, _ := testContainerInstance.Host(ctx)
	port, _ := testContainerInstance.MappedPort(ctx, "27017")
	endpoint := fmt.Sprintf("mongodb://%s:%s", host, port.Port())
	uniqueDBName := fmt.Sprintf("test_db_%d", time.Now().UnixNano())

	isolatedRepo, err := NewMongoRepository(ctx, Config{
		URI: endpoint, DBName: uniqueDBName, MaxPoolSize: 10, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to initialize isolated test repository: %v", err)
	}

	// ВАЖНО: Игнорируем ошибку шардинга на этапе настройки БД
	err = isolatedRepo.InitIndices(ctx)
	if err != nil {
		if !strings.Contains(err.Error(), "CommandNotFound") && !strings.Contains(err.Error(), "no such command") {
			t.Fatalf("failed to init indices: %v", err)
		}
	}

	teardown := func() {
		destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer destroyCancel()
		isolatedRepo.GetUnderlyingDB().Drop(destroyCtx)
		isolatedRepo.Close(destroyCtx)
	}

	return isolatedRepo, teardown
}

func TestNewMongoRepository_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	host, _ := testContainerInstance.Host(ctx)
	port, _ := testContainerInstance.MappedPort(ctx, "27017")
	endpoint := fmt.Sprintf("mongodb://%s:%s", host, port.Port())

	cfg := Config{
		URI:         endpoint,
		DBName:      "client_success_test_db",
		MaxPoolSize: 10,
		Timeout:     2 * time.Second,
	}

	repo, err := NewMongoRepository(ctx, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.GetUnderlyingDB())
	assert.NotNil(t, repo.collection) // Проверяем, что внутреннее поле коллекции проинициализировано

	assert.Equal(t, "client_success_test_db", repo.GetUnderlyingDB().Name())

	err = repo.Close(ctx)
	assert.NoError(t, err)
}

func TestNewMongoRepository_InvalidURI(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cfg := Config{
		URI:    "mongodb://invalid-uri-format:comma-in-port,123",
		DBName: "test_db",
	}

	repo, err := NewMongoRepository(ctx, cfg)
	assert.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to connect to mongo")
}

func TestNewMongoRepository_PingFailure_And_DefaultTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cfg := Config{
		URI:         "mongodb://localhost:59999",
		DBName:      "test_db",
		Timeout:     50 * time.Millisecond,
		MaxPoolSize: 5,
	}

	repo, err := NewMongoRepository(ctx, cfg)
	assert.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to ping mongo")
}

func TestMongoRepository_Close_Twice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	host, _ := testContainerInstance.Host(ctx)
	port, _ := testContainerInstance.MappedPort(ctx, "27017")
	endpoint := fmt.Sprintf("mongodb://%s:%s", host, port.Port())

	repo, err := NewMongoRepository(ctx, Config{
		URI:    endpoint,
		DBName: "client_close_test_db",
	})
	assert.NoError(t, err)

	err = repo.Close(ctx)
	assert.NoError(t, err)

	assert.NotPanics(t, func() {
		_ = repo.Close(ctx)
	})
}

func TestMongoRepository_GetUnderlyingDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	host, _ := testContainerInstance.Host(ctx)
	port, _ := testContainerInstance.MappedPort(ctx, "27017")
	endpoint := fmt.Sprintf("mongodb://%s:%s", host, port.Port())

	repo, err := NewMongoRepository(ctx, Config{
		URI:    endpoint,
		DBName: "getter_test_db",
	})
	assert.NoError(t, err)
	defer repo.Close(ctx)

	rawDB := repo.GetUnderlyingDB()
	assert.NotNil(t, rawDB)
	assert.IsType(t, &mongo.Database{}, rawDB)
}
