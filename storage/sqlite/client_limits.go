// Package sqlite предоставляет реализацию хранилища кастомных лимитов
// для Rate Limiter, используя базу данных SQLite.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	// Импортируем драйвер SQLite3. Пустой идентификатор (_) используется,
	// так как мы обращаемся к драйверу через интерфейс database/sql,
	// но пакет драйвера должен быть скомпилирован в бинарник.
	_ "github.com/mattn/go-sqlite3"
)

// SQL запросы для работы с таблицей лимитов.
const (
	// createTableSQL создает таблицу client_limits, если она не существует.
	// client_id: Уникальный идентификатор клиента (например, IP).
	// capacity: Емкость бакета для клиента.
	// rate: Скорость пополнения бакета (токенов/сек) для клиента.
	// updated_at: Время последнего обновления записи.
	createTableSQL = `
	CREATE TABLE IF NOT EXISTS client_limits (
		client_id TEXT PRIMARY KEY NOT NULL,
		capacity INTEGER NOT NULL,
		rate REAL NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	// getLimitSQL выбирает лимиты (capacity, rate) для заданного client_id.
	getLimitSQL = `SELECT capacity, rate FROM client_limits WHERE client_id = ?;`
	// setLimitSQL вставляет новую запись или обновляет существующую (UPSERT)
	// для заданного client_id с новыми значениями capacity и rate.
	setLimitSQL = `
	INSERT INTO client_limits (client_id, capacity, rate, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(client_id) DO UPDATE SET
		capacity = excluded.capacity,
		rate = excluded.rate,
		updated_at = CURRENT_TIMESTAMP;`
	deleteLimitSQL = `DELETE FROM client_limits WHERE client_id = ?;`
)

// SQLiteLimitStore реализует интерфейс ratelimiter.LimitProvider,
// используя базу данных SQLite для хранения и извлечения кастомных лимитов.
type SQLiteLimitStore struct {
	db *sql.DB // Указатель на объект соединения с базой данных SQLite.
}

// New создает и инициализирует новый SQLiteLimitStore.
// Открывает соединение с БД по указанному пути dbPath,
// проверяет соединение и создает таблицу client_limits, если она не существует.
// Возвращает созданный store или ошибку.
func New(dbPath string) (*SQLiteLimitStore, error) {
	log.Printf("INFO: Initializing SQLite limit store at %s", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database at %s: %w", dbPath, err)
	}
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create client_limits table: %w", err)
	}
	log.Printf("INFO: SQLite limit store initialized successfully.")
	return &SQLiteLimitStore{db: db}, nil
}

// GetLimit извлекает кастомные лимиты (capacity, rate) для заданного clientID из БД.
// Реализует метод интерфейса ratelimiter.LimitProvider.
// Возвращает capacity, rate и found=true, если лимит найден.
// Возвращает 0, 0 и found=false, если лимит не найден или произошла ошибка.
func (s *SQLiteLimitStore) GetLimit(clientID string) (capacity int64, rate float64, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	row := s.db.QueryRowContext(ctx, getLimitSQL, clientID)
	err := row.Scan(&capacity, &rate)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, false
		}
		log.Printf("ERROR: Failed to query limit for client %s: %v", clientID, err)
		return 0, 0, false
	}
	return capacity, rate, true
}

// SetLimit устанавливает или обновляет кастомные лимиты для заданного clientID в БД
func (s *SQLiteLimitStore) SetLimit(clientID string, capacity int64, rate float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := s.db.ExecContext(ctx, setLimitSQL, clientID, capacity, rate)
	if err != nil {
		log.Printf("ERROR: Failed to set limit for client %s (capacity=%d, rate=%.2f): %v", clientID, capacity, rate, err)
		return fmt.Errorf("failed to execute set limit statement: %w", err)
	}
	log.Printf("INFO: Set custom limit for client %s: capacity=%d, rate=%.2f/s", clientID, capacity, rate)
	return nil
}

// DeleteLimit удаляет кастомные лимиты для заданного clientID из БД.
// Реализует метод интерфейса ratelimiter.LimitManager.
func (s *SQLiteLimitStore) DeleteLimit(clientID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := s.db.ExecContext(ctx, deleteLimitSQL, clientID)
	if err != nil {
		log.Printf("ERROR: Failed to delete limit for client %s: %v", clientID, err)
		return fmt.Errorf("failed to execute delete limit statement: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("WARN: Could not get rows affected after deleting limit for client %s: %v", clientID, err)
	} else if rowsAffected == 0 {
		log.Printf("INFO: No custom limit found to delete for client %s", clientID)
	} else {
		log.Printf("INFO: Deleted custom limit for client %s", clientID)
	}

	return nil
}

// Closer закрывает соединение с базой данных SQLite.
// Реализует метод интерфейса ratelimiter.LimitProvider.
func (s *SQLiteLimitStore) Closer() error {
	if s.db != nil {
		log.Println("INFO: Closing SQLite limit store database connection.")
		return s.db.Close()
	}
	return nil
}
