package ackhandlernew

import (
	"github.com/lucas-clemente/quic-go/frames"
	"github.com/lucas-clemente/quic-go/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("receivedPacketHistory", func() {
	var (
		hist *receivedPacketHistory
	)

	BeforeEach(func() {
		hist = newReceivedPacketHistory()
	})

	Context("ranges", func() {
		It("adds the first packet", func() {
			hist.ReceivedPacket(4)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
		})

		It("doesn't care about duplicate packets", func() {
			hist.ReceivedPacket(4)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
		})

		It("adds a few consecutive packets", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(6)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 6}))
		})

		It("doesn't care about a duplicate packet contained in an existing range", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(6)
			hist.ReceivedPacket(5)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 6}))
		})

		It("extends a range at the front", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(3)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 3, End: 4}))
		})

		It("creates a new range when a packet is lost", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(6)
			Expect(hist.ranges.Len()).To(Equal(2))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 6, End: 6}))
		})

		It("creates a new range in between two ranges", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(10)
			Expect(hist.ranges.Len()).To(Equal(2))
			hist.ReceivedPacket(7)
			Expect(hist.ranges.Len()).To(Equal(3))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
			Expect(hist.ranges.Front().Next().Value).To(Equal(utils.PacketInterval{Start: 7, End: 7}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 10, End: 10}))
		})

		It("creates a new range before an existing range for a belated packet", func() {
			hist.ReceivedPacket(6)
			hist.ReceivedPacket(4)
			Expect(hist.ranges.Len()).To(Equal(2))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 6, End: 6}))
		})

		It("extends a previous range at the end", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(7)
			hist.ReceivedPacket(5)
			Expect(hist.ranges.Len()).To(Equal(2))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 5}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 7, End: 7}))
		})

		It("extends a range at the front", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(7)
			hist.ReceivedPacket(6)
			Expect(hist.ranges.Len()).To(Equal(2))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 6, End: 7}))
		})

		It("closes a range", func() {
			hist.ReceivedPacket(6)
			hist.ReceivedPacket(4)
			Expect(hist.ranges.Len()).To(Equal(2))
			hist.ReceivedPacket(5)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 6}))
		})

		It("closes a range in the middle", func() {
			hist.ReceivedPacket(1)
			hist.ReceivedPacket(10)
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(6)
			Expect(hist.ranges.Len()).To(Equal(4))
			hist.ReceivedPacket(5)
			Expect(hist.ranges.Len()).To(Equal(3))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 1, End: 1}))
			Expect(hist.ranges.Front().Next().Value).To(Equal(utils.PacketInterval{Start: 4, End: 6}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 10, End: 10}))
		})
	})

	Context("deleting", func() {
		It("does nothing when the history is empty", func() {
			hist.DeleteBelow(5)
			Expect(hist.ranges.Len()).To(BeZero())
		})

		It("deletes a range", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(10)
			hist.DeleteBelow(6)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 10, End: 10}))
		})

		It("deletes multiple ranges", func() {
			hist.ReceivedPacket(1)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(10)
			hist.DeleteBelow(8)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 10, End: 10}))
		})

		It("adjusts a range, if leastUnacked lies inside it", func() {
			hist.ReceivedPacket(3)
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(6)
			hist.DeleteBelow(4)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 6}))
		})

		It("adjusts a range, if leastUnacked is the last of the range", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(10)
			hist.DeleteBelow(5)
			Expect(hist.ranges.Len()).To(Equal(2))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 5, End: 5}))
			Expect(hist.ranges.Back().Value).To(Equal(utils.PacketInterval{Start: 10, End: 10}))
		})

		It("keeps a one-packet range, if leastUnacked is exactly that value", func() {
			hist.ReceivedPacket(4)
			hist.DeleteBelow(4)
			Expect(hist.ranges.Len()).To(Equal(1))
			Expect(hist.ranges.Front().Value).To(Equal(utils.PacketInterval{Start: 4, End: 4}))
		})
	})

	Context("ACK range export", func() {
		It("returns nil if there are no ranges", func() {
			Expect(hist.GetAckRanges()).To(BeNil())
		})

		It("gets a single ACK range", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			ackRanges := hist.GetAckRanges()
			Expect(ackRanges).To(HaveLen(1))
			Expect(ackRanges[0]).To(Equal(frames.AckRange{FirstPacketNumber: 4, LastPacketNumber: 5}))
		})

		It("gets multiple ACK ranges", func() {
			hist.ReceivedPacket(4)
			hist.ReceivedPacket(5)
			hist.ReceivedPacket(6)
			hist.ReceivedPacket(1)
			hist.ReceivedPacket(11)
			hist.ReceivedPacket(10)
			hist.ReceivedPacket(2)
			ackRanges := hist.GetAckRanges()
			Expect(ackRanges).To(HaveLen(3))
			Expect(ackRanges[0]).To(Equal(frames.AckRange{FirstPacketNumber: 10, LastPacketNumber: 11}))
			Expect(ackRanges[1]).To(Equal(frames.AckRange{FirstPacketNumber: 4, LastPacketNumber: 6}))
			Expect(ackRanges[2]).To(Equal(frames.AckRange{FirstPacketNumber: 1, LastPacketNumber: 2}))
		})
	})
})
