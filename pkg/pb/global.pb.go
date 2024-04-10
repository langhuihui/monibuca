// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.19.1
// source: global.proto

package pb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StreamSnapRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	StreamPath string `protobuf:"bytes,1,opt,name=streamPath,proto3" json:"streamPath,omitempty"`
}

func (x *StreamSnapRequest) Reset() {
	*x = StreamSnapRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamSnapRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamSnapRequest) ProtoMessage() {}

func (x *StreamSnapRequest) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamSnapRequest.ProtoReflect.Descriptor instead.
func (*StreamSnapRequest) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{0}
}

func (x *StreamSnapRequest) GetStreamPath() string {
	if x != nil {
		return x.StreamPath
	}
	return ""
}

type Wrap struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Timestamp uint32 `protobuf:"varint,1,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Size      uint32 `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
	Data      string `protobuf:"bytes,3,opt,name=data,proto3" json:"data,omitempty"`
}

func (x *Wrap) Reset() {
	*x = Wrap{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Wrap) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Wrap) ProtoMessage() {}

func (x *Wrap) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Wrap.ProtoReflect.Descriptor instead.
func (*Wrap) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{1}
}

func (x *Wrap) GetTimestamp() uint32 {
	if x != nil {
		return x.Timestamp
	}
	return 0
}

func (x *Wrap) GetSize() uint32 {
	if x != nil {
		return x.Size
	}
	return 0
}

func (x *Wrap) GetData() string {
	if x != nil {
		return x.Data
	}
	return ""
}

type TrackSnapShot struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Sequence  uint32 `protobuf:"varint,1,opt,name=sequence,proto3" json:"sequence,omitempty"`
	Timestamp uint32 `protobuf:"varint,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	WriteTime uint64 `protobuf:"varint,3,opt,name=writeTime,proto3" json:"writeTime,omitempty"`
	CanRead   bool   `protobuf:"varint,4,opt,name=canRead,proto3" json:"canRead,omitempty"`
	Wrap      *Wrap  `protobuf:"bytes,5,opt,name=wrap,proto3" json:"wrap,omitempty"`
}

func (x *TrackSnapShot) Reset() {
	*x = TrackSnapShot{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TrackSnapShot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TrackSnapShot) ProtoMessage() {}

func (x *TrackSnapShot) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TrackSnapShot.ProtoReflect.Descriptor instead.
func (*TrackSnapShot) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{2}
}

func (x *TrackSnapShot) GetSequence() uint32 {
	if x != nil {
		return x.Sequence
	}
	return 0
}

func (x *TrackSnapShot) GetTimestamp() uint32 {
	if x != nil {
		return x.Timestamp
	}
	return 0
}

func (x *TrackSnapShot) GetWriteTime() uint64 {
	if x != nil {
		return x.WriteTime
	}
	return 0
}

func (x *TrackSnapShot) GetCanRead() bool {
	if x != nil {
		return x.CanRead
	}
	return false
}

func (x *TrackSnapShot) GetWrap() *Wrap {
	if x != nil {
		return x.Wrap
	}
	return nil
}

type StreamSnapShot struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	VideoTrack []*TrackSnapShot `protobuf:"bytes,1,rep,name=videoTrack,proto3" json:"videoTrack,omitempty"`
	AudioTrack []*TrackSnapShot `protobuf:"bytes,2,rep,name=audioTrack,proto3" json:"audioTrack,omitempty"`
}

func (x *StreamSnapShot) Reset() {
	*x = StreamSnapShot{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamSnapShot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamSnapShot) ProtoMessage() {}

func (x *StreamSnapShot) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamSnapShot.ProtoReflect.Descriptor instead.
func (*StreamSnapShot) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{3}
}

func (x *StreamSnapShot) GetVideoTrack() []*TrackSnapShot {
	if x != nil {
		return x.VideoTrack
	}
	return nil
}

func (x *StreamSnapShot) GetAudioTrack() []*TrackSnapShot {
	if x != nil {
		return x.AudioTrack
	}
	return nil
}

type StopSubscribeRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id uint32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *StopSubscribeRequest) Reset() {
	*x = StopSubscribeRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StopSubscribeRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StopSubscribeRequest) ProtoMessage() {}

func (x *StopSubscribeRequest) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StopSubscribeRequest.ProtoReflect.Descriptor instead.
func (*StopSubscribeRequest) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{4}
}

func (x *StopSubscribeRequest) GetId() uint32 {
	if x != nil {
		return x.Id
	}
	return 0
}

type StopSubscribeResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Success bool `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
}

func (x *StopSubscribeResponse) Reset() {
	*x = StopSubscribeResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StopSubscribeResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StopSubscribeResponse) ProtoMessage() {}

func (x *StopSubscribeResponse) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StopSubscribeResponse.ProtoReflect.Descriptor instead.
func (*StopSubscribeResponse) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{5}
}

func (x *StopSubscribeResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

type RequestWithId struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id uint32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *RequestWithId) Reset() {
	*x = RequestWithId{}
	if protoimpl.UnsafeEnabled {
		mi := &file_global_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RequestWithId) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RequestWithId) ProtoMessage() {}

func (x *RequestWithId) ProtoReflect() protoreflect.Message {
	mi := &file_global_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RequestWithId.ProtoReflect.Descriptor instead.
func (*RequestWithId) Descriptor() ([]byte, []int) {
	return file_global_proto_rawDescGZIP(), []int{6}
}

func (x *RequestWithId) GetId() uint32 {
	if x != nil {
		return x.Id
	}
	return 0
}

var File_global_proto protoreflect.FileDescriptor

var file_global_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x67, 0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03,
	0x6d, 0x37, 0x73, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x33,
	0x0a, 0x11, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x6e, 0x61, 0x70, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x1e, 0x0a, 0x0a, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x50, 0x61, 0x74,
	0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x50,
	0x61, 0x74, 0x68, 0x22, 0x4c, 0x0a, 0x04, 0x57, 0x72, 0x61, 0x70, 0x12, 0x1c, 0x0a, 0x09, 0x74,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x09,
	0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x69, 0x7a,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x73, 0x69, 0x7a, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x64, 0x61, 0x74,
	0x61, 0x22, 0xa0, 0x01, 0x0a, 0x0d, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x53, 0x6e, 0x61, 0x70, 0x53,
	0x68, 0x6f, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x08, 0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x65, 0x12,
	0x1c, 0x0a, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0d, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x12, 0x1c, 0x0a,
	0x09, 0x77, 0x72, 0x69, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04,
	0x52, 0x09, 0x77, 0x72, 0x69, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x63,
	0x61, 0x6e, 0x52, 0x65, 0x61, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x63, 0x61,
	0x6e, 0x52, 0x65, 0x61, 0x64, 0x12, 0x1d, 0x0a, 0x04, 0x77, 0x72, 0x61, 0x70, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x57, 0x72, 0x61, 0x70, 0x52, 0x04,
	0x77, 0x72, 0x61, 0x70, 0x22, 0x78, 0x0a, 0x0e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x6e,
	0x61, 0x70, 0x53, 0x68, 0x6f, 0x74, 0x12, 0x32, 0x0a, 0x0a, 0x76, 0x69, 0x64, 0x65, 0x6f, 0x54,
	0x72, 0x61, 0x63, 0x6b, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x6d, 0x37, 0x73,
	0x2e, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x53, 0x6e, 0x61, 0x70, 0x53, 0x68, 0x6f, 0x74, 0x52, 0x0a,
	0x76, 0x69, 0x64, 0x65, 0x6f, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x12, 0x32, 0x0a, 0x0a, 0x61, 0x75,
	0x64, 0x69, 0x6f, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x12,
	0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x53, 0x6e, 0x61, 0x70, 0x53, 0x68,
	0x6f, 0x74, 0x52, 0x0a, 0x61, 0x75, 0x64, 0x69, 0x6f, 0x54, 0x72, 0x61, 0x63, 0x6b, 0x22, 0x26,
	0x0a, 0x14, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0d, 0x52, 0x02, 0x69, 0x64, 0x22, 0x31, 0x0a, 0x15, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x75,
	0x62, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x18, 0x0a, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x22, 0x1f, 0x0a, 0x0d, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x57, 0x69, 0x74, 0x68, 0x49, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x02, 0x69, 0x64, 0x32, 0x80, 0x03, 0x0a, 0x06, 0x47,
	0x6c, 0x6f, 0x62, 0x61, 0x6c, 0x12, 0x52, 0x0a, 0x08, 0x53, 0x68, 0x75, 0x74, 0x64, 0x6f, 0x77,
	0x6e, 0x12, 0x12, 0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x57,
	0x69, 0x74, 0x68, 0x49, 0x64, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x1a, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x14, 0x22, 0x12, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x68, 0x75, 0x74,
	0x64, 0x6f, 0x77, 0x6e, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x50, 0x0a, 0x07, 0x52, 0x65, 0x73,
	0x74, 0x61, 0x72, 0x74, 0x12, 0x12, 0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x57, 0x69, 0x74, 0x68, 0x49, 0x64, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79,
	0x22, 0x19, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x13, 0x22, 0x11, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x72,
	0x65, 0x73, 0x74, 0x61, 0x72, 0x74, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x63, 0x0a, 0x0a, 0x53,
	0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x6e, 0x61, 0x70, 0x12, 0x16, 0x2e, 0x6d, 0x37, 0x73, 0x2e,
	0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x6e, 0x61, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x13, 0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x6e,
	0x61, 0x70, 0x53, 0x68, 0x6f, 0x74, 0x22, 0x28, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x22, 0x12, 0x20,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x2f, 0x73, 0x6e, 0x61, 0x70,
	0x2f, 0x7b, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x50, 0x61, 0x74, 0x68, 0x3d, 0x2a, 0x2a, 0x7d,
	0x12, 0x6b, 0x0a, 0x0d, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x62,
	0x65, 0x12, 0x19, 0x2e, 0x6d, 0x37, 0x73, 0x2e, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x75, 0x62, 0x73,
	0x63, 0x72, 0x69, 0x62, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x6d,
	0x37, 0x73, 0x2e, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x23, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x1d,
	0x22, 0x18, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x74, 0x6f, 0x70, 0x2f, 0x73, 0x75, 0x62, 0x73,
	0x63, 0x72, 0x69, 0x62, 0x65, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x3a, 0x01, 0x2a, 0x42, 0x18, 0x5a,
	0x16, 0x6d, 0x37, 0x73, 0x2e, 0x6c, 0x69, 0x76, 0x65, 0x2f, 0x6d, 0x37, 0x73, 0x2f, 0x76, 0x35,
	0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_global_proto_rawDescOnce sync.Once
	file_global_proto_rawDescData = file_global_proto_rawDesc
)

func file_global_proto_rawDescGZIP() []byte {
	file_global_proto_rawDescOnce.Do(func() {
		file_global_proto_rawDescData = protoimpl.X.CompressGZIP(file_global_proto_rawDescData)
	})
	return file_global_proto_rawDescData
}

var file_global_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_global_proto_goTypes = []interface{}{
	(*StreamSnapRequest)(nil),     // 0: m7s.StreamSnapRequest
	(*Wrap)(nil),                  // 1: m7s.Wrap
	(*TrackSnapShot)(nil),         // 2: m7s.TrackSnapShot
	(*StreamSnapShot)(nil),        // 3: m7s.StreamSnapShot
	(*StopSubscribeRequest)(nil),  // 4: m7s.StopSubscribeRequest
	(*StopSubscribeResponse)(nil), // 5: m7s.StopSubscribeResponse
	(*RequestWithId)(nil),         // 6: m7s.RequestWithId
	(*emptypb.Empty)(nil),         // 7: google.protobuf.Empty
}
var file_global_proto_depIdxs = []int32{
	1, // 0: m7s.TrackSnapShot.wrap:type_name -> m7s.Wrap
	2, // 1: m7s.StreamSnapShot.videoTrack:type_name -> m7s.TrackSnapShot
	2, // 2: m7s.StreamSnapShot.audioTrack:type_name -> m7s.TrackSnapShot
	6, // 3: m7s.Global.Shutdown:input_type -> m7s.RequestWithId
	6, // 4: m7s.Global.Restart:input_type -> m7s.RequestWithId
	0, // 5: m7s.Global.StreamSnap:input_type -> m7s.StreamSnapRequest
	4, // 6: m7s.Global.StopSubscribe:input_type -> m7s.StopSubscribeRequest
	7, // 7: m7s.Global.Shutdown:output_type -> google.protobuf.Empty
	7, // 8: m7s.Global.Restart:output_type -> google.protobuf.Empty
	3, // 9: m7s.Global.StreamSnap:output_type -> m7s.StreamSnapShot
	5, // 10: m7s.Global.StopSubscribe:output_type -> m7s.StopSubscribeResponse
	7, // [7:11] is the sub-list for method output_type
	3, // [3:7] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_global_proto_init() }
func file_global_proto_init() {
	if File_global_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_global_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StreamSnapRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Wrap); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TrackSnapShot); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StreamSnapShot); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StopSubscribeRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StopSubscribeResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_global_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RequestWithId); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_global_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_global_proto_goTypes,
		DependencyIndexes: file_global_proto_depIdxs,
		MessageInfos:      file_global_proto_msgTypes,
	}.Build()
	File_global_proto = out.File
	file_global_proto_rawDesc = nil
	file_global_proto_goTypes = nil
	file_global_proto_depIdxs = nil
}
