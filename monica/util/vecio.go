package util

import (

	//"fmt"
	"io"
	"net"
	"os"
	"unsafe"
)

/*
#include <sys/uio.h>
// Structure for scatter/gather I/O.
struct iovec{
     void *iov_base; // Pointer to data.
     size_t iov_len; // Length of data.
};
*/

type SysIOVec struct {
	Base   uintptr
	Length uint64
}

type IOVec struct {
	Data   [][]byte
	Length int
	index  int
}

func (iov *IOVec) Append(b []byte) {
	iov.Data = append(iov.Data, b)
	iov.Length += len(b)
}

//  Data模型:
//  index -> | Data[0][0] | Data[0][1] | Data[0][2] | ... | Data[0][n] |
//			 | Data[1][0] | Data[1][1] | Data[1][2] | ... | Data[1][n] |
//				......
//			 | Data[n][0] | Data[n][1] | Data[n][2] | ... | Data[n][n] |
//
// index是下标

func (iov *IOVec) WriteTo(w io.Writer, n int) (written int, err error) {
	for n > 0 && iov.Length > 0 {
		data := iov.Data[iov.index]

		// 用来存放每次需要写入的数据
		var b []byte

		// 只会读n个字节,超出n个字节,不管
		// 如果iov.Data里面有1000个数据,可是每次只读184个字节,那么剩下的数据(856)重新放回Data
		if n > len(data) {
			b = data
		} else {
			b = data[:n]
		}

		// n个字节后面的数据
		// 如果这时候n个字节后面已经没有数据了,我们就将下标index往后移一位
		// 否则我们将n个字节后面的数据重新放回Data里.
		data = data[len(b):]
		if len(data) == 0 {
			iov.index++
		} else {
			iov.Data[iov.index] = data
		}

		n -= len(b)
		iov.Length -= len(b)
		written += len(b)

		if _, err = w.Write(b); err != nil {
			return
		}
	}
	return
}

type IOVecWriter struct {
	fd          uintptr
	smallBuffer []byte
	sysIOV      []SysIOVec
}

func NewIOVecWriter(w io.Writer) (iow *IOVecWriter) {
	var err error
	var file *os.File

	// TODO:是否要增加其他的类型断言
	switch value := w.(type) {
	case *net.TCPConn:
		{
			file, err = value.File()
			if err != nil {
				return
			}
		}
	case *os.File:
		{
			file = value
		}
	default:
		return
	}

	iow = &IOVecWriter{
		fd: file.Fd(),
	}

	return
}

//   1          2           3     4     5        6
//  ---   --------------   ---   ---   ---   -----------
// |   | |              | |   | |   | |   | |           | ......
//  ---   --------------   ---   ---   ---   -----------
//
// 1 -> 5个字节, 3 -> 15个字节, 4 -> 10个字节, 5 -> 15个字节

// 1,3,4,5内存块太小(小于16个字节),因此我们将它组装起来为samllbuffer
// 并且将Base置于每次组装smallBuffer前总长度的尾部.
//
// samllbuffer:
//  1   3   4   5   ........
//  ------------------------------
// |                              |
//  ------------------------------
//  <-->  第一个小内存块,假设地址为0xF10000
//    5
//  <------> 第二个小内存块,假设地址为0xF20000
//	   20
//  <----------> 第三个小内存块,假设地址为0xF30000
//       30
//  <--------------> 第四个小内存块,假设地址为0xF40000
//          45
//
// 开始Base == 每次组装smallBuffer尾部
// 即:
// Base1 = 0, smallBuffer += 5,
// Base3 = 5, smallBuffer += 15,
// Base4 = 20, smallBuffer += 10,
// Base5 = 30, smallBuffer += 15,
//
// 然后我们将每一块内存块都取出来,比samllBuffer小的内存块,我们就将Base指向内存块的地址
// 之前小于16个字节的内存块,肯定会比smallBuffer小,因为smallBuffer是所有小内存快的总和.
// 即:
// Base1 = &smallBuffer[0], Base1 = 0xF10000,
// Base3 = &smallBuffer[5], Base3 = 0xF20000,
// Base4 = &smallBuffer[20], Base4 = 0xF30000,
// Base5 = &smallBuffer[30], Base5 = 0xF40000,

func (iow *IOVecWriter) Write(data []byte) (written int, err error) {
	siov := SysIOVec{
		Length: uint64(len(data)),
	}

	// unsafe.Pointer == void *
	// Base 用整数的形式来记录内存中有几个数据
	// 如果数据小于16,这个时候小块内存的Base还不是数据的内存地址
	if siov.Length < 16 {
		// Base 置于上一块samllBuffer的末尾
		// 然后拼接smallBuffer
		siov.Base = uintptr(len(iow.smallBuffer))
		iow.smallBuffer = append(iow.smallBuffer, data...)
	} else {
		siov.Base = uintptr(unsafe.Pointer(&data[0]))
	}

	iow.sysIOV = append(iow.sysIOV, siov)

	return written, nil
}

func (iow *IOVecWriter) Flush() error {
	// 取出每一块内存
	for i, _ := range iow.sysIOV {
		siov := &iow.sysIOV[i] // 一定要拿地址,如果这里不是取地址,那么无法改变下面Base的值
		if siov.Base < uintptr(len(iow.smallBuffer)) {
			// 这个时候小块内存的Base就是数据的内存地址
			siov.Base = uintptr(unsafe.Pointer(&iow.smallBuffer[siov.Base]))
		}
	}

	N := 1024
	count := len(iow.sysIOV)
	// 每次最多取1024个内存块(不管是大内存块,还是小内存块)
	for i := 0; i < count; i += N {
		n := count - i
		if n > N {
			n = N
		}

		// _, _, errno := syscall.Syscall(syscall.SYS_WRITEV, iow.fd, uintptr(unsafe.Pointer(&iow.sysIOV[i])), uintptr(n))
		// if errno != 0 {
		// 	return errors.New(errno.Error())
		// }
	}

	iow.sysIOV = iow.sysIOV[:0]
	iow.smallBuffer = iow.smallBuffer[:0]

	return nil
}
