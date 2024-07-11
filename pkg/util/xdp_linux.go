package util

import (
	"fmt"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"reflect"
	"syscall"
	"time"
	"unsafe"
)

// DefaultSocketOptions is the default SocketOptions used by an xdp.Socket created without specifying options.
var DefaultSocketOptions = SocketOptions{
	NumFrames:              128,
	FrameSize:              2048,
	FillRingNumDescs:       64,
	CompletionRingNumDescs: 64,
	RxRingNumDescs:         64,
	TxRingNumDescs:         64,
}

type umemRing struct {
	Producer *uint32
	Consumer *uint32
	Descs    []uint64
}

type rxTxRing struct {
	Producer *uint32
	Consumer *uint32
	Descs    []Desc
}

// A Socket is an implementation of the AF_XDP Linux socket type for reading packets from a device.
type Socket struct {
	fd                       int
	umem                     []byte
	fillRing                 umemRing
	rxRing                   rxTxRing
	txRing                   rxTxRing
	completionRing           umemRing
	qidconfMap               *ebpf.Map
	xsksMap                  *ebpf.Map
	program                  *ebpf.Program
	ifindex                  int
	numTransmitted           int
	numFilled                int
	freeRXDescs, freeTXDescs []bool
	options                  SocketOptions
	rxDescs                  []Desc
	getTXDescs, getRXDescs   []Desc
}

// SocketOptions are configuration settings used to bind an XDP socket.
type SocketOptions struct {
	NumFrames              int
	FrameSize              int
	FillRingNumDescs       int
	CompletionRingNumDescs int
	RxRingNumDescs         int
	TxRingNumDescs         int
}

// Desc represents an XDP Rx/Tx descriptor.
type Desc unix.XDPDesc

// Stats contains various counters of the XDP socket, such as numbers of
// sent/received frames.
type Stats struct {
	// Filled is the number of items consumed thus far by the Linux kernel
	// from the Fill ring queue.
	Filled uint64

	// Received is the number of items consumed thus far by the user of
	// this package from the Rx ring queue.
	Received uint64

	// Transmitted is the number of items consumed thus far by the Linux
	// kernel from the Tx ring queue.
	Transmitted uint64

	// Completed is the number of items consumed thus far by the user of
	// this package from the Completion ring queue.
	Completed uint64

	// KernelStats contains the in-kernel statistics of the corresponding
	// XDP socket, such as the number of invalid descriptors that were
	// submitted into Fill or Tx ring queues.
	KernelStats unix.XDPStatistics
}

// DefaultSocketFlags are the flags which are passed to bind(2) system call
// when the XDP socket is bound, possible values include unix.XDP_SHARED_UMEM,
// unix.XDP_COPY, unix.XDP_ZEROCOPY.
var DefaultSocketFlags uint16

// DefaultXdpFlags are the flags which are passed when the XDP program is
// attached to the network link, possible values include
// unix.XDP_FLAGS_DRV_MODE, unix.XDP_FLAGS_HW_MODE, unix.XDP_FLAGS_SKB_MODE,
// unix.XDP_FLAGS_UPDATE_IF_NOEXIST.
var DefaultXdpFlags uint32

func init() {
	DefaultSocketFlags = 0
	DefaultXdpFlags = 0
}

// NewSocket returns a new XDP socket attached to the network interface which
// has the given interface, and attached to the given queue on that network
// interface.
func NewSocket(Ifindex int, QueueID int, options *SocketOptions) (xsk *Socket, err error) {
	if options == nil {
		options = &DefaultSocketOptions
	}
	xsk = &Socket{fd: -1, ifindex: Ifindex, options: *options}

	xsk.fd, err = syscall.Socket(unix.AF_XDP, syscall.SOCK_RAW, 0)
	if err != nil {
		return nil, fmt.Errorf("syscall.Socket failed: %v", err)
	}

	xsk.umem, err = syscall.Mmap(-1, 0, options.NumFrames*options.FrameSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANONYMOUS|syscall.MAP_POPULATE)
	if err != nil {
		xsk.Close()
		return nil, fmt.Errorf("syscall.Mmap failed: %v", err)
	}

	xdpUmemReg := unix.XDPUmemReg{
		Addr:     uint64(uintptr(unsafe.Pointer(&xsk.umem[0]))),
		Len:      uint64(len(xsk.umem)),
		Size:     uint32(options.FrameSize),
		Headroom: 0,
	}

	var errno syscall.Errno
	var rc uintptr

	rc, _, errno = unix.Syscall6(syscall.SYS_SETSOCKOPT, uintptr(xsk.fd),
		unix.SOL_XDP, unix.XDP_UMEM_REG,
		uintptr(unsafe.Pointer(&xdpUmemReg)),
		unsafe.Sizeof(xdpUmemReg), 0)
	if rc != 0 {
		xsk.Close()
		return nil, fmt.Errorf("unix.SetsockoptUint64 XDP_UMEM_REG failed: %v", errno)
	}

	err = syscall.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_UMEM_FILL_RING,
		options.FillRingNumDescs)
	if err != nil {
		xsk.Close()
		return nil, fmt.Errorf("unix.SetsockoptUint64 XDP_UMEM_FILL_RING failed: %v", err)
	}

	err = unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_UMEM_COMPLETION_RING,
		options.CompletionRingNumDescs)
	if err != nil {
		xsk.Close()
		return nil, fmt.Errorf("unix.SetsockoptUint64 XDP_UMEM_COMPLETION_RING failed: %v", err)
	}

	var rxRing bool
	if options.RxRingNumDescs > 0 {
		err = unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_RX_RING,
			options.RxRingNumDescs)
		if err != nil {
			xsk.Close()
			return nil, fmt.Errorf("unix.SetsockoptUint64 XDP_RX_RING failed: %v", err)
		}
		rxRing = true
	}

	var txRing bool
	if options.TxRingNumDescs > 0 {
		err = unix.SetsockoptInt(xsk.fd, unix.SOL_XDP, unix.XDP_TX_RING,
			options.TxRingNumDescs)
		if err != nil {
			xsk.Close()
			return nil, fmt.Errorf("unix.SetsockoptUint64 XDP_TX_RING failed: %v", err)
		}
		txRing = true
	}

	if !(rxRing || txRing) {
		return nil, fmt.Errorf("RxRingNumDescs and TxRingNumDescs cannot both be set to zero")
	}

	var offsets unix.XDPMmapOffsets
	var vallen uint32
	vallen = uint32(unsafe.Sizeof(offsets))
	rc, _, errno = unix.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(xsk.fd),
		unix.SOL_XDP, unix.XDP_MMAP_OFFSETS,
		uintptr(unsafe.Pointer(&offsets)),
		uintptr(unsafe.Pointer(&vallen)), 0)
	if rc != 0 {
		xsk.Close()
		return nil, fmt.Errorf("unix.Syscall6 getsockopt XDP_MMAP_OFFSETS failed: %v", errno)
	}

	fillRingSlice, err := syscall.Mmap(xsk.fd, unix.XDP_UMEM_PGOFF_FILL_RING,
		int(offsets.Fr.Desc+uint64(options.FillRingNumDescs)*uint64(unsafe.Sizeof(uint64(0)))),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		xsk.Close()
		return nil, fmt.Errorf("syscall.Mmap XDP_UMEM_PGOFF_FILL_RING failed: %v", err)
	}

	xsk.fillRing.Producer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&fillRingSlice[0])) + uintptr(offsets.Fr.Producer)))
	xsk.fillRing.Consumer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&fillRingSlice[0])) + uintptr(offsets.Fr.Consumer)))
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&xsk.fillRing.Descs))
	sh.Data = uintptr(unsafe.Pointer(&fillRingSlice[0])) + uintptr(offsets.Fr.Desc)
	sh.Len = options.FillRingNumDescs
	sh.Cap = options.FillRingNumDescs

	completionRingSlice, err := syscall.Mmap(xsk.fd, unix.XDP_UMEM_PGOFF_COMPLETION_RING,
		int(offsets.Cr.Desc+uint64(options.CompletionRingNumDescs)*uint64(unsafe.Sizeof(uint64(0)))),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		xsk.Close()
		return nil, fmt.Errorf("syscall.Mmap XDP_UMEM_PGOFF_COMPLETION_RING failed: %v", err)
	}

	xsk.completionRing.Producer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&completionRingSlice[0])) + uintptr(offsets.Cr.Producer)))
	xsk.completionRing.Consumer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&completionRingSlice[0])) + uintptr(offsets.Cr.Consumer)))
	sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.completionRing.Descs))
	sh.Data = uintptr(unsafe.Pointer(&completionRingSlice[0])) + uintptr(offsets.Cr.Desc)
	sh.Len = options.CompletionRingNumDescs
	sh.Cap = options.CompletionRingNumDescs

	if rxRing {
		rxRingSlice, err := syscall.Mmap(xsk.fd, unix.XDP_PGOFF_RX_RING,
			int(offsets.Rx.Desc+uint64(options.RxRingNumDescs)*uint64(unsafe.Sizeof(Desc{}))),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_POPULATE)
		if err != nil {
			xsk.Close()
			return nil, fmt.Errorf("syscall.Mmap XDP_PGOFF_RX_RING failed: %v", err)
		}

		xsk.rxRing.Producer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&rxRingSlice[0])) + uintptr(offsets.Rx.Producer)))
		xsk.rxRing.Consumer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&rxRingSlice[0])) + uintptr(offsets.Rx.Consumer)))
		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.rxRing.Descs))
		sh.Data = uintptr(unsafe.Pointer(&rxRingSlice[0])) + uintptr(offsets.Rx.Desc)
		sh.Len = options.RxRingNumDescs
		sh.Cap = options.RxRingNumDescs

		xsk.rxDescs = make([]Desc, 0, options.RxRingNumDescs)
	}

	if txRing {
		txRingSlice, err := syscall.Mmap(xsk.fd, unix.XDP_PGOFF_TX_RING,
			int(offsets.Tx.Desc+uint64(options.TxRingNumDescs)*uint64(unsafe.Sizeof(Desc{}))),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_POPULATE)
		if err != nil {
			xsk.Close()
			return nil, fmt.Errorf("syscall.Mmap XDP_PGOFF_TX_RING failed: %v", err)
		}

		xsk.txRing.Producer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&txRingSlice[0])) + uintptr(offsets.Tx.Producer)))
		xsk.txRing.Consumer = (*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&txRingSlice[0])) + uintptr(offsets.Tx.Consumer)))
		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.txRing.Descs))
		sh.Data = uintptr(unsafe.Pointer(&txRingSlice[0])) + uintptr(offsets.Tx.Desc)
		sh.Len = options.TxRingNumDescs
		sh.Cap = options.TxRingNumDescs
	}

	sa := unix.SockaddrXDP{
		Flags:   DefaultSocketFlags,
		Ifindex: uint32(Ifindex),
		QueueID: uint32(QueueID),
	}
	if err = unix.Bind(xsk.fd, &sa); err != nil {
		xsk.Close()
		return nil, fmt.Errorf("syscall.Bind SockaddrXDP failed: %v", err)
	}

	xsk.freeRXDescs = make([]bool, options.NumFrames)
	xsk.freeTXDescs = make([]bool, options.NumFrames)
	for i := range xsk.freeRXDescs {
		xsk.freeRXDescs[i] = true
	}
	for i := range xsk.freeTXDescs {
		xsk.freeTXDescs[i] = true
	}
	xsk.getTXDescs = make([]Desc, 0, options.CompletionRingNumDescs)
	xsk.getRXDescs = make([]Desc, 0, options.FillRingNumDescs)

	return xsk, nil
}

// Fill submits the given descriptors to be filled (i.e. to receive frames into)
// it returns how many descriptors where actually put onto Fill ring queue.
// The descriptors can be acquired either by calling the GetDescs() method or
// by calling Receive() method.
func (xsk *Socket) Fill(descs []Desc) int {
	numFreeSlots := xsk.NumFreeFillSlots()
	if numFreeSlots < len(descs) {
		descs = descs[:numFreeSlots]
	}

	prod := *xsk.fillRing.Producer
	for _, desc := range descs {
		xsk.fillRing.Descs[prod&uint32(xsk.options.FillRingNumDescs-1)] = desc.Addr
		prod++
		xsk.freeRXDescs[desc.Addr/uint64(xsk.options.FrameSize)] = false
	}
	//fencer.SFence()
	*xsk.fillRing.Producer = prod

	xsk.numFilled += len(descs)

	return len(descs)
}

// Receive returns the descriptors which were filled, i.e. into which frames
// were received into.
func (xsk *Socket) Receive(num int) []Desc {
	numAvailable := xsk.NumReceived()
	if num > int(numAvailable) {
		num = int(numAvailable)
	}

	descs := xsk.rxDescs[:0]
	cons := *xsk.rxRing.Consumer
	//fencer.LFence()
	for i := 0; i < num; i++ {
		descs = append(descs, xsk.rxRing.Descs[cons&uint32(xsk.options.RxRingNumDescs-1)])
		cons++
		xsk.freeRXDescs[descs[i].Addr/uint64(xsk.options.FrameSize)] = true
	}
	//fencer.MFence()
	*xsk.rxRing.Consumer = cons

	xsk.numFilled -= len(descs)

	return descs
}

// Transmit submits the given descriptors to be sent out, it returns how many
// descriptors were actually pushed onto the Tx ring queue.
// The descriptors can be acquired either by calling the GetDescs() method or
// by calling Receive() method.
func (xsk *Socket) Transmit(descs []Desc) (numSubmitted int) {
	numFreeSlots := xsk.NumFreeTxSlots()
	if len(descs) > numFreeSlots {
		descs = descs[:numFreeSlots]
	}

	prod := *xsk.txRing.Producer
	for _, desc := range descs {
		xsk.txRing.Descs[prod&uint32(xsk.options.TxRingNumDescs-1)] = desc
		prod++
		xsk.freeTXDescs[desc.Addr/uint64(xsk.options.FrameSize)] = false
	}
	//fencer.SFence()
	*xsk.txRing.Producer = prod

	xsk.numTransmitted += len(descs)

	numSubmitted = len(descs)

	var rc uintptr
	var errno syscall.Errno
	for {
		rc, _, errno = unix.Syscall6(syscall.SYS_SENDTO,
			uintptr(xsk.fd),
			0, 0,
			uintptr(unix.MSG_DONTWAIT),
			0, 0)
		if rc != 0 {
			switch errno {
			case unix.EINTR:
				// try again
			case unix.EAGAIN:
				return
			case unix.EBUSY: // "completed but not sent"
				return
			default:
				panic(fmt.Errorf("sendto failed with rc=%d and errno=%d", rc, errno))
			}
		} else {
			break
		}
	}

	return
}

// FD returns the file descriptor associated with this xdp.Socket which can be
// used e.g. to do polling.
func (xsk *Socket) FD() int {
	return xsk.fd
}

// Poll blocks until kernel informs us that it has either received
// or completed (i.e. actually sent) some frames that were previously submitted
// using Fill() or Transmit() methods.
// The numReceived return value can be used as the argument for subsequent
// Receive() method call.
func (xsk *Socket) Poll(timeout int) (numReceived int, numCompleted int, err error) {
	var events int16
	if xsk.numFilled > 0 {
		events |= unix.POLLIN
	}
	if xsk.numTransmitted > 0 {
		events |= unix.POLLOUT
	}
	if events == 0 {
		return
	}

	var pfds [1]unix.PollFd
	pfds[0].Fd = int32(xsk.fd)
	pfds[0].Events = events
	for err = unix.EINTR; err == unix.EINTR; {
		_, err = unix.Poll(pfds[:], timeout)
	}
	if err != nil {
		return 0, 0, err
	}

	numReceived = xsk.NumReceived()
	if numCompleted = xsk.NumCompleted(); numCompleted > 0 {
		xsk.Complete(numCompleted)
	}

	return
}

// GetDescs returns up to n descriptors which are not currently in use.
// if rx is true, return desc in first half of umem, 2nd half otherwise
func (xsk *Socket) GetDescs(n int, rx bool) []Desc {
	if n > cap(xsk.getRXDescs) {
		n = cap(xsk.getRXDescs)
	}
	if !rx {
		if n > cap(xsk.getTXDescs) {
			n = cap(xsk.getTXDescs)
		}
	}
	// numOfUMEMChunks := len(xsk.freeRXDescs) / 2
	// if n > numOfUMEMChunks {
	// 	n = numOfUMEMChunks
	// }

	descs := xsk.getRXDescs[:0]
	j := 0
	start := 0
	end := cap(xsk.getRXDescs)
	freeList := xsk.freeRXDescs
	if !rx {
		start = cap(xsk.getRXDescs)
		end = len(xsk.freeTXDescs)
		freeList = xsk.freeTXDescs
		descs = xsk.getTXDescs[:0]
	}
	for i := start; i < end && j < n; i++ {
		if freeList[i] == true {
			descs = append(descs, Desc{
				Addr: uint64(i) * uint64(xsk.options.FrameSize),
				Len:  uint32(xsk.options.FrameSize),
			})
			j++
		}
	}
	return descs
}

// GetFrame returns the buffer containing the frame corresponding to the given
// descriptor. The returned byte slice points to the actual buffer of the
// corresponding frame, so modiyfing this slice modifies the frame contents.
func (xsk *Socket) GetFrame(d Desc) []byte {
	return xsk.umem[d.Addr : d.Addr+uint64(d.Len)]
}

// Close closes and frees the resources allocated by the Socket.
func (xsk *Socket) Close() error {
	allErrors := []error{}
	var err error

	if xsk.fd != -1 {
		if err = unix.Close(xsk.fd); err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to close XDP socket: %v", err))
		}
		xsk.fd = -1

		var sh *reflect.SliceHeader

		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.completionRing.Descs))
		sh.Data = uintptr(0)
		sh.Len = 0
		sh.Cap = 0

		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.txRing.Descs))
		sh.Data = uintptr(0)
		sh.Len = 0
		sh.Cap = 0

		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.rxRing.Descs))
		sh.Data = uintptr(0)
		sh.Len = 0
		sh.Cap = 0

		sh = (*reflect.SliceHeader)(unsafe.Pointer(&xsk.fillRing.Descs))
		sh.Data = uintptr(0)
		sh.Len = 0
		sh.Cap = 0
	}

	if xsk.umem != nil {
		if err := syscall.Munmap(xsk.umem); err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to unmap the UMEM: %v", err))
		}
		xsk.umem = nil
	}

	if len(allErrors) > 0 {
		return allErrors[0]
	}

	return nil
}

// Complete consumes up to n descriptors from the Completion ring queue to
// which the kernel produces when it has actually transmitted a descriptor it
// got from Tx ring queue.
// You should use this method if you are doing polling on the xdp.Socket file
// descriptor yourself, rather than using the Poll() method.
func (xsk *Socket) Complete(n int) {
	cons := *xsk.completionRing.Consumer
	//fencer.LFence()
	for i := 0; i < n; i++ {
		addr := xsk.completionRing.Descs[cons&uint32(xsk.options.CompletionRingNumDescs-1)]
		cons++
		xsk.freeTXDescs[addr/uint64(xsk.options.FrameSize)] = true
	}
	//fencer.MFence()
	*xsk.completionRing.Consumer = cons

	xsk.numTransmitted -= n
}

// NumFreeFillSlots returns how many free slots are available on the Fill ring
// queue, i.e. the queue to which we produce descriptors which should be filled
// by the kernel with incoming frames.
func (xsk *Socket) NumFreeFillSlots() int {
	prod := *xsk.fillRing.Producer
	cons := *xsk.fillRing.Consumer
	max := uint32(xsk.options.FillRingNumDescs)

	n := max - (prod - cons)
	if n > max {
		n = max
	}

	return int(n)
}

// NumFreeTxSlots returns how many free slots are available on the Tx ring
// queue, i.e. the queue to which we produce descriptors which should be
// transmitted by the kernel to the wire.
func (xsk *Socket) NumFreeTxSlots() int {
	prod := *xsk.txRing.Producer
	cons := *xsk.txRing.Consumer
	max := uint32(xsk.options.TxRingNumDescs)

	n := max - (prod - cons)
	if n > max {
		n = max
	}

	return int(n)
}

// NumReceived returns how many descriptors are there on the Rx ring queue
// which were produced by the kernel and which we have not yet consumed.
func (xsk *Socket) NumReceived() int {
	prod := *xsk.rxRing.Producer
	cons := *xsk.rxRing.Consumer
	max := uint32(xsk.options.RxRingNumDescs)

	n := prod - cons
	if n > max {
		n = max
	}

	return int(n)
}

// NumCompleted returns how many descriptors are there on the Completion ring
// queue which were produced by the kernel and which we have not yet consumed.
func (xsk *Socket) NumCompleted() int {
	prod := *xsk.completionRing.Producer
	cons := *xsk.completionRing.Consumer
	max := uint32(xsk.options.CompletionRingNumDescs)

	n := prod - cons
	if n > max {
		n = max
	}

	return int(n)
}

// NumFilled returns how many descriptors are there on the Fill ring
// queue which have not yet been consumed by the kernel.
// This method is useful if you're polling the xdp.Socket file descriptor
// yourself, rather than using the Poll() method - if it returns a number
// greater than zero it means you should set the unix.POLLIN flag.
func (xsk *Socket) NumFilled() int {
	return xsk.numFilled
}

// NumTransmitted returns how many descriptors are there on the Tx ring queue
// which have not yet been consumed by the kernel.
// Note that even after the descriptors are consumed by the kernel from the Tx
// ring queue, it doesn't mean that they have actually been sent out on the
// wire, that can be assumed only after the descriptors have been produced by
// the kernel to the Completion ring queue.
// This method is useful if you're polling the xdp.Socket file descriptor
// yourself, rather than using the Poll() method - if it returns a number
// greater than zero it means you should set the unix.POLLOUT flag.
func (xsk *Socket) NumTransmitted() int {
	return xsk.numTransmitted
}

// Stats returns various statistics for this XDP socket.
func (xsk *Socket) Stats() (Stats, error) {
	var stats Stats
	var size uint64

	stats.Filled = uint64(*xsk.fillRing.Consumer)
	stats.Received = uint64(*xsk.rxRing.Consumer)
	if xsk.txRing.Consumer != nil {
		stats.Transmitted = uint64(*xsk.txRing.Consumer)
	}
	if xsk.completionRing.Consumer != nil {
		stats.Completed = uint64(*xsk.completionRing.Consumer)
	}
	size = uint64(unsafe.Sizeof(stats.KernelStats))
	rc, _, errno := unix.Syscall6(syscall.SYS_GETSOCKOPT,
		uintptr(xsk.fd),
		unix.SOL_XDP, unix.XDP_STATISTICS,
		uintptr(unsafe.Pointer(&stats.KernelStats)),
		uintptr(unsafe.Pointer(&size)), 0)
	if rc != 0 {
		return stats, fmt.Errorf("getsockopt XDP_STATISTICS failed with errno %d", errno)
	}
	return stats, nil
}

// Program represents the necessary data structures for a simple XDP program that can filter traffic
// based on the attached rx queue.
type Program struct {
	Program *ebpf.Program
	Queues  *ebpf.Map
	Sockets *ebpf.Map
}

// Attach the XDP Program to an interface.
func (p *Program) Attach(Ifindex int) error {
	if err := removeProgram(Ifindex); err != nil {
		return err
	}
	return attachProgram(Ifindex, p.Program)
}

// Detach the XDP Program from an interface.
func (p *Program) Detach(Ifindex int) error {
	return removeProgram(Ifindex)
}

// Register adds the socket file descriptor as the recipient for packets from the given queueID.
func (p *Program) Register(queueID int, fd int) error {
	if err := p.Sockets.Put(uint32(queueID), uint32(fd)); err != nil {
		return fmt.Errorf("failed to update xsksMap: %v", err)
	}

	if err := p.Queues.Put(uint32(queueID), uint32(1)); err != nil {
		return fmt.Errorf("failed to update qidconfMap: %v", err)
	}
	return nil
}

// Unregister removes any associated mapping to sockets for the given queueID.
func (p *Program) Unregister(queueID int) error {
	if err := p.Queues.Delete(uint32(queueID)); err != nil {
		return err
	}
	if err := p.Sockets.Delete(uint32(queueID)); err != nil {
		return err
	}
	return nil
}

// Close closes and frees the resources allocated for the Program.
func (p *Program) Close() error {
	allErrors := []error{}
	if p.Sockets != nil {
		if err := p.Sockets.Close(); err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to close xsksMap: %v", err))
		}
		p.Sockets = nil
	}

	if p.Queues != nil {
		if err := p.Queues.Close(); err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to close qidconfMap: %v", err))
		}
		p.Queues = nil
	}

	if p.Program != nil {
		if err := p.Program.Close(); err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to close XDP program: %v", err))
		}
		p.Program = nil
	}

	if len(allErrors) > 0 {
		return allErrors[0]
	}
	return nil
}

// NewProgram returns a translation of the default eBPF XDP program found in the
// xsk_load_xdp_prog() function in <linux>/tools/lib/bpf/xsk.c:
// https://github.com/torvalds/linux/blob/master/tools/lib/bpf/xsk.c#L259
func NewProgram(maxQueueEntries int) (*Program, error) {
	qidconfMap, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       "qidconf_map",
		Type:       ebpf.Array,
		KeySize:    uint32(unsafe.Sizeof(int32(0))),
		ValueSize:  uint32(unsafe.Sizeof(int32(0))),
		MaxEntries: uint32(maxQueueEntries),
		Flags:      0,
		InnerMap:   nil,
	})
	if err != nil {
		return nil, fmt.Errorf("ebpf.NewMap qidconf_map failed (try increasing RLIMIT_MEMLOCK): %v", err)
	}

	xsksMap, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       "xsks_map",
		Type:       ebpf.XSKMap,
		KeySize:    uint32(unsafe.Sizeof(int32(0))),
		ValueSize:  uint32(unsafe.Sizeof(int32(0))),
		MaxEntries: uint32(maxQueueEntries),
		Flags:      0,
		InnerMap:   nil,
	})
	if err != nil {
		return nil, fmt.Errorf("ebpf.NewMap xsks_map failed (try increasing RLIMIT_MEMLOCK): %v", err)
	}

	/*
		This is a translation of the default eBPF XDP program found in the
		xsk_load_xdp_prog() function in <linux>/tools/lib/bpf/xsk.c:
		https://github.com/torvalds/linux/blob/master/tools/lib/bpf/xsk.c#L259

		// This is the C-program:
		// SEC("xdp_sock") int xdp_sock_prog(struct xdp_md *ctx)
		// {
		//     int *qidconf, index = ctx->rx_queue_index;
		//
		//     // A set entry here means that the correspnding queue_id
		//     // has an active AF_XDP socket bound to it.
		//     qidconf = bpf_map_lookup_elem(&qidconf_map, &index);
		//     if (!qidconf)
		//         return XDP_ABORTED;
		//
		//     if (*qidconf)
		//         return bpf_redirect_map(&xsks_map, index, 0);
		//
		//     return XDP_PASS;
		// }
		//
		struct bpf_insn prog[] = {
			// r1 = *(u32 *)(r1 + 16)
			BPF_LDX_MEM(BPF_W, BPF_REG_1, BPF_REG_1, 16),   // 0
			// *(u32 *)(r10 - 4) = r1
			BPF_STX_MEM(BPF_W, BPF_REG_10, BPF_REG_1, -4),  // 1
			BPF_MOV64_REG(BPF_REG_2, BPF_REG_10),           // 2
			BPF_ALU64_IMM(BPF_ADD, BPF_REG_2, -4),          // 3
			BPF_LD_MAP_FD(BPF_REG_1, xsk->qidconf_map_fd),  // 4 (2 instructions)
			BPF_EMIT_CALL(BPF_FUNC_map_lookup_elem),        // 5
			BPF_MOV64_REG(BPF_REG_1, BPF_REG_0),            // 6
			BPF_MOV32_IMM(BPF_REG_0, 0),                    // 7
			// if r1 == 0 goto +8
			BPF_JMP_IMM(BPF_JEQ, BPF_REG_1, 0, 8),          // 8
			BPF_MOV32_IMM(BPF_REG_0, 2),                    // 9
			// r1 = *(u32 *)(r1 + 0)
			BPF_LDX_MEM(BPF_W, BPF_REG_1, BPF_REG_1, 0),    // 10
			// if r1 == 0 goto +5
			BPF_JMP_IMM(BPF_JEQ, BPF_REG_1, 0, 5),          // 11
			// r2 = *(u32 *)(r10 - 4)
			BPF_LD_MAP_FD(BPF_REG_1, xsk->xsks_map_fd),     // 12 (2 instructions)
			BPF_LDX_MEM(BPF_W, BPF_REG_2, BPF_REG_10, -4),  // 13
			BPF_MOV32_IMM(BPF_REG_3, 0),                    // 14
			BPF_EMIT_CALL(BPF_FUNC_redirect_map),           // 15
			// The jumps are to this instruction
			BPF_EXIT_INSN(),                                // 16
		};

		eBPF instructions:
		  0: code: 97 dst_reg: 1 src_reg: 1 off: 16 imm: 0   // 0
		  1: code: 99 dst_reg: 10 src_reg: 1 off: -4 imm: 0  // 1
		  2: code: 191 dst_reg: 2 src_reg: 10 off: 0 imm: 0  // 2
		  3: code: 7 dst_reg: 2 src_reg: 0 off: 0 imm: -4    // 3
		  4: code: 24 dst_reg: 1 src_reg: 1 off: 0 imm: 4    // 4 XXX use qidconfMap.FD as IMM
		  5: code: 0 dst_reg: 0 src_reg: 0 off: 0 imm: 0     //   part of the same instruction
		  6: code: 133 dst_reg: 0 src_reg: 0 off: 0 imm: 1   // 5
		  7: code: 191 dst_reg: 1 src_reg: 0 off: 0 imm: 0   // 6
		  8: code: 180 dst_reg: 0 src_reg: 0 off: 0 imm: 0   // 7
		  9: code: 21 dst_reg: 1 src_reg: 0 off: 8 imm: 0    // 8
		  10: code: 180 dst_reg: 0 src_reg: 0 off: 0 imm: 2  // 9
		  11: code: 97 dst_reg: 1 src_reg: 1 off: 0 imm: 0   // 10
		  12: code: 21 dst_reg: 1 src_reg: 0 off: 5 imm: 0   // 11
		  13: code: 24 dst_reg: 1 src_reg: 1 off: 0 imm: 5   // 12 XXX use xsksMap.FD as IMM
		  14: code: 0 dst_reg: 0 src_reg: 0 off: 0 imm: 0    //    part of the same instruction
		  15: code: 97 dst_reg: 2 src_reg: 10 off: -4 imm: 0 // 13
		  16: code: 180 dst_reg: 3 src_reg: 0 off: 0 imm: 0  // 14
		  17: code: 133 dst_reg: 0 src_reg: 0 off: 0 imm: 51 // 15
		  18: code: 149 dst_reg: 0 src_reg: 0 off: 0 imm: 0  // 16
	*/

	program, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Name: "xsk_ebpf",
		Type: ebpf.XDP,
		Instructions: asm.Instructions{
			{OpCode: 97, Dst: 1, Src: 1, Offset: 16},                                  // 0: code: 97 dst_reg: 1 src_reg: 1 off: 16 imm: 0   // 0
			{OpCode: 99, Dst: 10, Src: 1, Offset: -4},                                 // 1: code: 99 dst_reg: 10 src_reg: 1 off: -4 imm: 0  // 1
			{OpCode: 191, Dst: 2, Src: 10},                                            // 2: code: 191 dst_reg: 2 src_reg: 10 off: 0 imm: 0  // 2
			{OpCode: 7, Dst: 2, Src: 0, Offset: 0, Constant: -4},                      // 3: code: 7 dst_reg: 2 src_reg: 0 off: 0 imm: -4    // 3
			{OpCode: 24, Dst: 1, Src: 1, Offset: 0, Constant: int64(qidconfMap.FD())}, // 4: code: 24 dst_reg: 1 src_reg: 1 off: 0 imm: 4    // 4 XXX use qidconfMap.FD as IMM
			//{ OpCode: 0 },                                                                 // 5: code: 0 dst_reg: 0 src_reg: 0 off: 0 imm: 0     //   part of the same instruction
			{OpCode: 133, Dst: 0, Src: 0, Constant: 1},                  // 6: code: 133 dst_reg: 0 src_reg: 0 off: 0 imm: 1   // 5
			{OpCode: 191, Dst: 1, Src: 0},                               // 7: code: 191 dst_reg: 1 src_reg: 0 off: 0 imm: 0   // 6
			{OpCode: 180, Dst: 0, Src: 0},                               // 8: code: 180 dst_reg: 0 src_reg: 0 off: 0 imm: 0   // 7
			{OpCode: 21, Dst: 1, Src: 0, Offset: 8},                     // 9: code: 21 dst_reg: 1 src_reg: 0 off: 8 imm: 0    // 8
			{OpCode: 180, Dst: 0, Src: 0, Constant: 2},                  // 10: code: 180 dst_reg: 0 src_reg: 0 off: 0 imm: 2  // 9
			{OpCode: 97, Dst: 1, Src: 1},                                // 11: code: 97 dst_reg: 1 src_reg: 1 off: 0 imm: 0   // 10
			{OpCode: 21, Dst: 1, Offset: 5},                             // 12: code: 21 dst_reg: 1 src_reg: 0 off: 5 imm: 0   // 11
			{OpCode: 24, Dst: 1, Src: 1, Constant: int64(xsksMap.FD())}, // 13: code: 24 dst_reg: 1 src_reg: 1 off: 0 imm: 5   // 12 XXX use xsksMap.FD as IMM
			//{ OpCode: 0 },                                                                 // 14: code: 0 dst_reg: 0 src_reg: 0 off: 0 imm: 0    //    part of the same instruction
			{OpCode: 97, Dst: 2, Src: 10, Offset: -4}, // 15: code: 97 dst_reg: 2 src_reg: 10 off: -4 imm: 0 // 13
			{OpCode: 180, Dst: 3},                     // 16: code: 180 dst_reg: 3 src_reg: 0 off: 0 imm: 0  // 14
			{OpCode: 133, Constant: 51},               // 17: code: 133 dst_reg: 0 src_reg: 0 off: 0 imm: 51 // 15
			{OpCode: 149},                             // 18: code: 149 dst_reg: 0 src_reg: 0 off: 0 imm: 0  // 16
		},
		License:       "LGPL-2.1 or BSD-2-Clause",
		KernelVersion: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("error: ebpf.NewProgram failed: %v", err)
	}

	return &Program{Program: program, Queues: qidconfMap, Sockets: xsksMap}, nil
}

// LoadProgram load a external XDP program, along with queue and socket map;
// fname is the BPF kernel program file (.o);
// funcname is the function name in the program file;
// qidmapname is the Queues map name;
// xskmapname is the Sockets map name;
func LoadProgram(fname, funcname, qidmapname, xskmapname string) (*Program, error) {
	prog := new(Program)
	col, err := ebpf.LoadCollection(fname)
	if err != nil {
		return nil, err
	}
	var ok bool
	if prog.Program, ok = col.Programs[funcname]; !ok {
		return nil, fmt.Errorf("%v doesn't contain a function named %v", fname, funcname)
	}
	if prog.Queues, ok = col.Maps[qidmapname]; !ok {
		return nil, fmt.Errorf("%v doesn't contain a queue map named %v", fname, qidmapname)
	}
	if prog.Sockets, ok = col.Maps[xskmapname]; !ok {
		return nil, fmt.Errorf("%v doesn't contain a socket map named %v", fname, xskmapname)
	}
	return prog, nil
}

// removeProgram removes an existing XDP program from the given network interface.
func removeProgram(Ifindex int) error {
	var link netlink.Link
	var err error
	link, err = netlink.LinkByIndex(Ifindex)
	if err != nil {
		return err
	}
	if !isXdpAttached(link) {
		return nil
	}
	if err = netlink.LinkSetXdpFd(link, -1); err != nil {
		return fmt.Errorf("netlink.LinkSetXdpFd(link, -1) failed: %v", err)
	}
	for {
		link, err = netlink.LinkByIndex(Ifindex)
		if err != nil {
			return err
		}
		if !isXdpAttached(link) {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func isXdpAttached(link netlink.Link) bool {
	if link.Attrs() != nil && link.Attrs().Xdp != nil && link.Attrs().Xdp.Attached {
		return true
	}
	return false
}

// attachProgram attaches the given XDP program to the network interface.
func attachProgram(Ifindex int, program *ebpf.Program) error {
	link, err := netlink.LinkByIndex(Ifindex)
	if err != nil {
		return err
	}
	return netlink.LinkSetXdpFdWithFlags(link, program.FD(), int(DefaultXdpFlags))
}
