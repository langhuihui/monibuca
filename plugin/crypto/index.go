package plugin_crypto

import (
	m7s "m7s.live/v5"
	crypto "m7s.live/v5/plugin/crypto/pkg"
)

var _ = m7s.InstallPlugin[CryptoPlugin](m7s.PluginMeta{
	NewTransformer: crypto.NewTransform,
})

type CryptoPlugin struct {
	m7s.Plugin
}
