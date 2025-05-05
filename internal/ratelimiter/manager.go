package ratelimiter

// LimitManager определяет интерфейс для управления кастомными лимитами клиентов.
// Этот интерфейс используется компонентами, отвечающими за администрирование лимитов (например, Admin API).
type LimitManager interface {
	// GetLimit получает текущие лимиты для клиента.
	GetLimit(clientID string) (capacity int64, rate float64, found bool)
	// SetLimit устанавливает или обновляет лимиты для клиента.
	SetLimit(clientID string, capacity int64, rate float64) error
	// DeleteLimit удаляет кастомные лимиты для клиента.
	// После удаления будут использоваться лимиты по умолчанию.
	DeleteLimit(clientID string) error
	// Возможно, в будущем: ListLimits() ([]ClientLimit, error)
}

// Примечание: Closer() не включен сюда, так как закрытие ресурсов (БД)
// управляется на уровне инициализации LimitProvider в main.
