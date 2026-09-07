package domain

import (
	"time"

	"github.com/google/uuid"
)

// MenuCategory категория меню заведения вместе с позициями
type MenuCategory struct {
	ID        uuid.UUID
	Name      string
	SortOrder int
	Items     []MenuItem
}

// MenuItem позиция меню
//
// ExternalID идентификатор позиции в учётной системе заведения, по нему идёт
// синхронизация меню. Внутри платформы позиция адресуется полем ID
//
// StockQty = nil означает, что заведение не ведёт остатки по этой позиции
// её можно заказывать в любом количестве, пока IsAvailable
type MenuItem struct {
	ID          uuid.UUID
	CategoryID  uuid.UUID
	ExternalID  string
	Name        string
	Description string
	Price       int64
	IsAvailable bool
	StockQty    *int
	ImageURL    *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Orderable сообщает, можно ли заказать qty единиц позиции прямо сейчас
func (i MenuItem) Orderable(qty int) bool {
	if !i.IsAvailable || qty <= 0 {
		return false
	}
	if i.StockQty == nil {
		return true
	}
	return *i.StockQty >= qty
}

// InStock сообщает, доступна ли позиция хотя бы в одном экземпляре
func (i MenuItem) InStock() bool {
	return i.Orderable(1)
}

// Available возвращает доступный остаток: -1, если остатки не ведутся
func (i MenuItem) Available() int {
	if i.StockQty == nil {
		return -1
	}
	return *i.StockQty
}

// MenuSync полное меню, присланное заведением для синхронизации
type MenuSync struct {
	Categories []MenuSyncCategory
}

// MenuSyncCategory категория в запросе синхронизации
type MenuSyncCategory struct {
	Name      string
	SortOrder int
	Items     []MenuSyncItem
}

// MenuSyncItem позиция в запросе синхронизации
type MenuSyncItem struct {
	ExternalID  string
	Name        string
	Description string
	Price       int64
	IsAvailable bool
	StockQty    *int
	ImageURL    *string
}

// MenuSyncResult итог синхронизации меню
//
// ItemsDisabled позиции, которых не было в присланном меню
type MenuSyncResult struct {
	CategoriesTotal int
	ItemsCreated    int
	ItemsUpdated    int
	ItemsDisabled   int
}

// MenuItemUpdate точечное изменение позиции заведением
type MenuItemUpdate struct {
	IsAvailable *bool
	StockQty    **int
	Price       *int64
}

// Validate проверяет присланное меню на пригодность к синхронизации
func (m MenuSync) Validate() error {
	if len(m.Categories) == 0 {
		return Invalid("Меню не содержит категорий")
	}

	seenCategories := make(map[string]struct{}, len(m.Categories))
	seenItems := make(map[string]struct{})

	for _, category := range m.Categories {
		if category.Name == "" {
			return Invalid("Категория без названия", Detail{
				Code: CodeValidation, Message: "Название категории обязательно", Field: "categories[].name",
			})
		}
		if _, dup := seenCategories[category.Name]; dup {
			return Invalid("Категории дублируются", Detail{
				Code: CodeValidation, Message: "Дубликат категории: " + category.Name, Field: "categories[].name",
			})
		}
		seenCategories[category.Name] = struct{}{}

		for _, item := range category.Items {
			if item.ExternalID == "" {
				return Invalid("Позиция без external_id", Detail{
					Code: CodeValidation, Message: "external_id обязателен", Field: "categories[].items[].external_id",
				})
			}
			if _, dup := seenItems[item.ExternalID]; dup {
				return Invalid("Позиции дублируются", Detail{
					Code:    CodeValidation,
					Message: "Дубликат external_id: " + item.ExternalID,
					Field:   "categories[].items[].external_id",
				})
			}
			seenItems[item.ExternalID] = struct{}{}

			if item.Price < 0 {
				return Invalid("Отрицательная цена", Detail{
					Code: CodeValidation, Message: "Цена не может быть отрицательной", Field: "categories[].items[].price",
				})
			}
			if item.StockQty != nil && *item.StockQty < 0 {
				return Invalid("Отрицательный остаток", Detail{
					Code: CodeValidation, Message: "Остаток не может быть отрицательным", Field: "categories[].items[].stock_qty",
				})
			}
		}
	}
	return nil
}
