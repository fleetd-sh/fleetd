// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.1
// 	protoc        (unknown)
// source: update/v1/update.proto

package updatev1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type UpdatePackage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id          string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Version     string                 `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
	ReleaseDate *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=release_date,json=releaseDate,proto3" json:"release_date,omitempty"`
	ChangeLog   string                 `protobuf:"bytes,4,opt,name=change_log,json=changeLog,proto3" json:"change_log,omitempty"`
	FileUrl     string                 `protobuf:"bytes,5,opt,name=file_url,json=fileUrl,proto3" json:"file_url,omitempty"`
	DeviceTypes []string               `protobuf:"bytes,6,rep,name=device_types,json=deviceTypes,proto3" json:"device_types,omitempty"`
}

func (x *UpdatePackage) Reset() {
	*x = UpdatePackage{}
	mi := &file_update_v1_update_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UpdatePackage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UpdatePackage) ProtoMessage() {}

func (x *UpdatePackage) ProtoReflect() protoreflect.Message {
	mi := &file_update_v1_update_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UpdatePackage.ProtoReflect.Descriptor instead.
func (*UpdatePackage) Descriptor() ([]byte, []int) {
	return file_update_v1_update_proto_rawDescGZIP(), []int{0}
}

func (x *UpdatePackage) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *UpdatePackage) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

func (x *UpdatePackage) GetReleaseDate() *timestamppb.Timestamp {
	if x != nil {
		return x.ReleaseDate
	}
	return nil
}

func (x *UpdatePackage) GetChangeLog() string {
	if x != nil {
		return x.ChangeLog
	}
	return ""
}

func (x *UpdatePackage) GetFileUrl() string {
	if x != nil {
		return x.FileUrl
	}
	return ""
}

func (x *UpdatePackage) GetDeviceTypes() []string {
	if x != nil {
		return x.DeviceTypes
	}
	return nil
}

type CreateUpdatePackageRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id          string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Version     string                 `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
	ReleaseDate *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=release_date,json=releaseDate,proto3" json:"release_date,omitempty"`
	ChangeLog   string                 `protobuf:"bytes,4,opt,name=change_log,json=changeLog,proto3" json:"change_log,omitempty"`
	FileUrl     string                 `protobuf:"bytes,5,opt,name=file_url,json=fileUrl,proto3" json:"file_url,omitempty"`
	DeviceTypes []string               `protobuf:"bytes,6,rep,name=device_types,json=deviceTypes,proto3" json:"device_types,omitempty"`
}

func (x *CreateUpdatePackageRequest) Reset() {
	*x = CreateUpdatePackageRequest{}
	mi := &file_update_v1_update_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CreateUpdatePackageRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateUpdatePackageRequest) ProtoMessage() {}

func (x *CreateUpdatePackageRequest) ProtoReflect() protoreflect.Message {
	mi := &file_update_v1_update_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateUpdatePackageRequest.ProtoReflect.Descriptor instead.
func (*CreateUpdatePackageRequest) Descriptor() ([]byte, []int) {
	return file_update_v1_update_proto_rawDescGZIP(), []int{1}
}

func (x *CreateUpdatePackageRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *CreateUpdatePackageRequest) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

func (x *CreateUpdatePackageRequest) GetReleaseDate() *timestamppb.Timestamp {
	if x != nil {
		return x.ReleaseDate
	}
	return nil
}

func (x *CreateUpdatePackageRequest) GetChangeLog() string {
	if x != nil {
		return x.ChangeLog
	}
	return ""
}

func (x *CreateUpdatePackageRequest) GetFileUrl() string {
	if x != nil {
		return x.FileUrl
	}
	return ""
}

func (x *CreateUpdatePackageRequest) GetDeviceTypes() []string {
	if x != nil {
		return x.DeviceTypes
	}
	return nil
}

type CreateUpdatePackageResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *CreateUpdatePackageResponse) Reset() {
	*x = CreateUpdatePackageResponse{}
	mi := &file_update_v1_update_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CreateUpdatePackageResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateUpdatePackageResponse) ProtoMessage() {}

func (x *CreateUpdatePackageResponse) ProtoReflect() protoreflect.Message {
	mi := &file_update_v1_update_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateUpdatePackageResponse.ProtoReflect.Descriptor instead.
func (*CreateUpdatePackageResponse) Descriptor() ([]byte, []int) {
	return file_update_v1_update_proto_rawDescGZIP(), []int{2}
}

func (x *CreateUpdatePackageResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *CreateUpdatePackageResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type GetAvailableUpdatesRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DeviceType     string                 `protobuf:"bytes,1,opt,name=device_type,json=deviceType,proto3" json:"device_type,omitempty"`
	LastUpdateDate *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=last_update_date,json=lastUpdateDate,proto3" json:"last_update_date,omitempty"`
}

func (x *GetAvailableUpdatesRequest) Reset() {
	*x = GetAvailableUpdatesRequest{}
	mi := &file_update_v1_update_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAvailableUpdatesRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAvailableUpdatesRequest) ProtoMessage() {}

func (x *GetAvailableUpdatesRequest) ProtoReflect() protoreflect.Message {
	mi := &file_update_v1_update_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAvailableUpdatesRequest.ProtoReflect.Descriptor instead.
func (*GetAvailableUpdatesRequest) Descriptor() ([]byte, []int) {
	return file_update_v1_update_proto_rawDescGZIP(), []int{3}
}

func (x *GetAvailableUpdatesRequest) GetDeviceType() string {
	if x != nil {
		return x.DeviceType
	}
	return ""
}

func (x *GetAvailableUpdatesRequest) GetLastUpdateDate() *timestamppb.Timestamp {
	if x != nil {
		return x.LastUpdateDate
	}
	return nil
}

type GetAvailableUpdatesResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Updates []*UpdatePackage `protobuf:"bytes,1,rep,name=updates,proto3" json:"updates,omitempty"`
}

func (x *GetAvailableUpdatesResponse) Reset() {
	*x = GetAvailableUpdatesResponse{}
	mi := &file_update_v1_update_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAvailableUpdatesResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAvailableUpdatesResponse) ProtoMessage() {}

func (x *GetAvailableUpdatesResponse) ProtoReflect() protoreflect.Message {
	mi := &file_update_v1_update_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAvailableUpdatesResponse.ProtoReflect.Descriptor instead.
func (*GetAvailableUpdatesResponse) Descriptor() ([]byte, []int) {
	return file_update_v1_update_proto_rawDescGZIP(), []int{4}
}

func (x *GetAvailableUpdatesResponse) GetUpdates() []*UpdatePackage {
	if x != nil {
		return x.Updates
	}
	return nil
}

var File_update_v1_update_proto protoreflect.FileDescriptor

var file_update_v1_update_proto_rawDesc = []byte{
	0x0a, 0x16, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2f, 0x76, 0x31, 0x2f, 0x75, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x09, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65,
	0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0xd5, 0x01, 0x0a, 0x0d, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50,
	0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f,
	0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e,
	0x12, 0x3d, 0x0a, 0x0c, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65, 0x5f, 0x64, 0x61, 0x74, 0x65,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61,
	0x6d, 0x70, 0x52, 0x0b, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65, 0x44, 0x61, 0x74, 0x65, 0x12,
	0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x5f, 0x6c, 0x6f, 0x67, 0x18, 0x04, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x4c, 0x6f, 0x67, 0x12, 0x19,
	0x0a, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x07, 0x66, 0x69, 0x6c, 0x65, 0x55, 0x72, 0x6c, 0x12, 0x21, 0x0a, 0x0c, 0x64, 0x65, 0x76,
	0x69, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x73, 0x22, 0xe2, 0x01, 0x0a,
	0x1a, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50, 0x61, 0x63,
	0x6b, 0x61, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x76,
	0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x76, 0x65,
	0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x3d, 0x0a, 0x0c, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65,
	0x5f, 0x64, 0x61, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x0b, 0x72, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65,
	0x44, 0x61, 0x74, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x5f, 0x6c,
	0x6f, 0x67, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65,
	0x4c, 0x6f, 0x67, 0x12, 0x19, 0x0a, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x75, 0x72, 0x6c, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x66, 0x69, 0x6c, 0x65, 0x55, 0x72, 0x6c, 0x12, 0x21,
	0x0a, 0x0c, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x73, 0x18, 0x06,
	0x20, 0x03, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65,
	0x73, 0x22, 0x51, 0x0a, 0x1b, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74,
	0x65, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x18, 0x0a, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x22, 0x83, 0x01, 0x0a, 0x1a, 0x47, 0x65, 0x74, 0x41, 0x76, 0x61, 0x69,
	0x6c, 0x61, 0x62, 0x6c, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x1f, 0x0a, 0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x74, 0x79,
	0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65,
	0x54, 0x79, 0x70, 0x65, 0x12, 0x44, 0x0a, 0x10, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x75, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x5f, 0x64, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x0e, 0x6c, 0x61, 0x73, 0x74,
	0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x44, 0x61, 0x74, 0x65, 0x22, 0x51, 0x0a, 0x1b, 0x47, 0x65,
	0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65,
	0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x32, 0x0a, 0x07, 0x75, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x75, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50, 0x61, 0x63,
	0x6b, 0x61, 0x67, 0x65, 0x52, 0x07, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x73, 0x32, 0xdb, 0x01,
	0x0a, 0x0d, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12,
	0x64, 0x0a, 0x13, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50,
	0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x12, 0x25, 0x2e, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2e,
	0x76, 0x31, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50,
	0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x26, 0x2e,
	0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65,
	0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x64, 0x0a, 0x13, 0x47, 0x65, 0x74, 0x41, 0x76, 0x61, 0x69,
	0x6c, 0x61, 0x62, 0x6c, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x73, 0x12, 0x25, 0x2e, 0x75,
	0x70, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x41, 0x76, 0x61, 0x69,
	0x6c, 0x61, 0x62, 0x6c, 0x65, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x26, 0x2e, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e,
	0x47, 0x65, 0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x55, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x92, 0x01, 0x0a, 0x0d,
	0x63, 0x6f, 0x6d, 0x2e, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x42, 0x0b, 0x55,
	0x70, 0x64, 0x61, 0x74, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x2f, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x66, 0x6c, 0x65, 0x65, 0x74, 0x2d, 0x73,
	0x68, 0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x75, 0x70, 0x64, 0x61, 0x74,
	0x65, 0x2f, 0x76, 0x31, 0x3b, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x76, 0x31, 0xa2, 0x02, 0x03,
	0x55, 0x58, 0x58, 0xaa, 0x02, 0x09, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x56, 0x31, 0xca,
	0x02, 0x09, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x15, 0x55, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0xea, 0x02, 0x0a, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x3a, 0x3a, 0x56, 0x31,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_update_v1_update_proto_rawDescOnce sync.Once
	file_update_v1_update_proto_rawDescData = file_update_v1_update_proto_rawDesc
)

func file_update_v1_update_proto_rawDescGZIP() []byte {
	file_update_v1_update_proto_rawDescOnce.Do(func() {
		file_update_v1_update_proto_rawDescData = protoimpl.X.CompressGZIP(file_update_v1_update_proto_rawDescData)
	})
	return file_update_v1_update_proto_rawDescData
}

var file_update_v1_update_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_update_v1_update_proto_goTypes = []any{
	(*UpdatePackage)(nil),               // 0: update.v1.UpdatePackage
	(*CreateUpdatePackageRequest)(nil),  // 1: update.v1.CreateUpdatePackageRequest
	(*CreateUpdatePackageResponse)(nil), // 2: update.v1.CreateUpdatePackageResponse
	(*GetAvailableUpdatesRequest)(nil),  // 3: update.v1.GetAvailableUpdatesRequest
	(*GetAvailableUpdatesResponse)(nil), // 4: update.v1.GetAvailableUpdatesResponse
	(*timestamppb.Timestamp)(nil),       // 5: google.protobuf.Timestamp
}
var file_update_v1_update_proto_depIdxs = []int32{
	5, // 0: update.v1.UpdatePackage.release_date:type_name -> google.protobuf.Timestamp
	5, // 1: update.v1.CreateUpdatePackageRequest.release_date:type_name -> google.protobuf.Timestamp
	5, // 2: update.v1.GetAvailableUpdatesRequest.last_update_date:type_name -> google.protobuf.Timestamp
	0, // 3: update.v1.GetAvailableUpdatesResponse.updates:type_name -> update.v1.UpdatePackage
	1, // 4: update.v1.UpdateService.CreateUpdatePackage:input_type -> update.v1.CreateUpdatePackageRequest
	3, // 5: update.v1.UpdateService.GetAvailableUpdates:input_type -> update.v1.GetAvailableUpdatesRequest
	2, // 6: update.v1.UpdateService.CreateUpdatePackage:output_type -> update.v1.CreateUpdatePackageResponse
	4, // 7: update.v1.UpdateService.GetAvailableUpdates:output_type -> update.v1.GetAvailableUpdatesResponse
	6, // [6:8] is the sub-list for method output_type
	4, // [4:6] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_update_v1_update_proto_init() }
func file_update_v1_update_proto_init() {
	if File_update_v1_update_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_update_v1_update_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_update_v1_update_proto_goTypes,
		DependencyIndexes: file_update_v1_update_proto_depIdxs,
		MessageInfos:      file_update_v1_update_proto_msgTypes,
	}.Build()
	File_update_v1_update_proto = out.File
	file_update_v1_update_proto_rawDesc = nil
	file_update_v1_update_proto_goTypes = nil
	file_update_v1_update_proto_depIdxs = nil
}