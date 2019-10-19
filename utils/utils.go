package utils

import (
	ma "github.com/multiformats/go-multiaddr"
)

// ByString can sort multiaddresses by its string
type ByString []ma.Multiaddr

func (m ByString) Len() int           { return len(m) }
func (m ByString) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m ByString) Less(i, j int) bool { return m[i].String() < m[j].String() }
