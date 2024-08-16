package cascade

import (
	"github.com/quic-go/quic-go"
	"gorm.io/gorm"
)

type Instance struct {
	gorm.Model
	Name            string
	Secret          string `gorm:"unique;index:idx_secret"`
	IP              string
	Online          bool
	quic.Connection `gorm:"-"`
}

func (i *Instance) GetKey() uint {
	return i.ID
}
