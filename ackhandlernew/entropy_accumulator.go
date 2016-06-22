package ackhandlernew

import "github.com/lucas-clemente/quic-go/protocol"

// EntropyAccumulator accumulates the entropy according to the QUIC docs
type EntropyAccumulator byte

// Add the contribution of the entropy flag of a given packet number
func (e *EntropyAccumulator) Add(packetNumber protocol.PacketNumber, entropyFlag bool) {
	if entropyFlag {
		(*e) ^= 0x01 << (packetNumber % 8)
	}
}

// Subtract the contribution of the entropy flag of a given packet number
func (e *EntropyAccumulator) Subtract(packetNumber protocol.PacketNumber, entropyFlag bool) {
	e.Add(packetNumber, entropyFlag)
}

// Get the byte of entropy
func (e *EntropyAccumulator) Get() byte {
	return byte(*e)
}
