package secondaryports

import (
	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
)

type StoragePort interface {
	Save(msg dto.Message, t msgstore.EntryType) error
	Latest(n int) ([]msgstore.Entry, error)
}
