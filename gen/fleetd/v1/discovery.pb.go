// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.1
// 	protoc        (unknown)
// source: fleetd/v1/discovery.proto

package fleetpb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type GetDeviceInfoRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetDeviceInfoRequest) Reset() {
	*x = GetDeviceInfoRequest{}
	mi := &file_fleetd_v1_discovery_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetDeviceInfoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetDeviceInfoRequest) ProtoMessage() {}

func (x *GetDeviceInfoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fleetd_v1_discovery_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetDeviceInfoRequest.ProtoReflect.Descriptor instead.
func (*GetDeviceInfoRequest) Descriptor() ([]byte, []int) {
	return file_fleetd_v1_discovery_proto_rawDescGZIP(), []int{0}
}

type GetDeviceInfoResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DeviceId   string `protobuf:"bytes,1,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`       // Local device ID (e.g. MAC address)
	Configured bool   `protobuf:"varint,2,opt,name=configured,proto3" json:"configured,omitempty"`                  // Whether device is registered with fleet server
	DeviceType string `protobuf:"bytes,3,opt,name=device_type,json=deviceType,proto3" json:"device_type,omitempty"` // Device hardware type
	Version    string `protobuf:"bytes,4,opt,name=version,proto3" json:"version,omitempty"`                         // Current software version
}

func (x *GetDeviceInfoResponse) Reset() {
	*x = GetDeviceInfoResponse{}
	mi := &file_fleetd_v1_discovery_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetDeviceInfoResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetDeviceInfoResponse) ProtoMessage() {}

func (x *GetDeviceInfoResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fleetd_v1_discovery_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetDeviceInfoResponse.ProtoReflect.Descriptor instead.
func (*GetDeviceInfoResponse) Descriptor() ([]byte, []int) {
	return file_fleetd_v1_discovery_proto_rawDescGZIP(), []int{1}
}

func (x *GetDeviceInfoResponse) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *GetDeviceInfoResponse) GetConfigured() bool {
	if x != nil {
		return x.Configured
	}
	return false
}

func (x *GetDeviceInfoResponse) GetDeviceType() string {
	if x != nil {
		return x.DeviceType
	}
	return ""
}

func (x *GetDeviceInfoResponse) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

type ConfigureDeviceRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DeviceName  string `protobuf:"bytes,1,opt,name=device_name,json=deviceName,proto3" json:"device_name,omitempty"`    // Human-readable device name
	ApiEndpoint string `protobuf:"bytes,2,opt,name=api_endpoint,json=apiEndpoint,proto3" json:"api_endpoint,omitempty"` // Fleet server endpoint URL
}

func (x *ConfigureDeviceRequest) Reset() {
	*x = ConfigureDeviceRequest{}
	mi := &file_fleetd_v1_discovery_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConfigureDeviceRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConfigureDeviceRequest) ProtoMessage() {}

func (x *ConfigureDeviceRequest) ProtoReflect() protoreflect.Message {
	mi := &file_fleetd_v1_discovery_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConfigureDeviceRequest.ProtoReflect.Descriptor instead.
func (*ConfigureDeviceRequest) Descriptor() ([]byte, []int) {
	return file_fleetd_v1_discovery_proto_rawDescGZIP(), []int{2}
}

func (x *ConfigureDeviceRequest) GetDeviceName() string {
	if x != nil {
		return x.DeviceName
	}
	return ""
}

func (x *ConfigureDeviceRequest) GetApiEndpoint() string {
	if x != nil {
		return x.ApiEndpoint
	}
	return ""
}

type ConfigureDeviceResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Success  bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message  string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	DeviceId string `protobuf:"bytes,3,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"` // Server-assigned device ID
	ApiKey   string `protobuf:"bytes,4,opt,name=api_key,json=apiKey,proto3" json:"api_key,omitempty"`       // API key for future authentication
}

func (x *ConfigureDeviceResponse) Reset() {
	*x = ConfigureDeviceResponse{}
	mi := &file_fleetd_v1_discovery_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConfigureDeviceResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConfigureDeviceResponse) ProtoMessage() {}

func (x *ConfigureDeviceResponse) ProtoReflect() protoreflect.Message {
	mi := &file_fleetd_v1_discovery_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConfigureDeviceResponse.ProtoReflect.Descriptor instead.
func (*ConfigureDeviceResponse) Descriptor() ([]byte, []int) {
	return file_fleetd_v1_discovery_proto_rawDescGZIP(), []int{3}
}

func (x *ConfigureDeviceResponse) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *ConfigureDeviceResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *ConfigureDeviceResponse) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *ConfigureDeviceResponse) GetApiKey() string {
	if x != nil {
		return x.ApiKey
	}
	return ""
}

var File_fleetd_v1_discovery_proto protoreflect.FileDescriptor

var file_fleetd_v1_discovery_proto_rawDesc = []byte{
	0x0a, 0x19, 0x66, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x2f, 0x76, 0x31, 0x2f, 0x64, 0x69, 0x73, 0x63,
	0x6f, 0x76, 0x65, 0x72, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x09, 0x66, 0x6c, 0x65,
	0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x22, 0x16, 0x0a, 0x14, 0x47, 0x65, 0x74, 0x44, 0x65, 0x76,
	0x69, 0x63, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x8f,
	0x01, 0x0a, 0x15, 0x47, 0x65, 0x74, 0x44, 0x65, 0x76, 0x69, 0x63, 0x65, 0x49, 0x6e, 0x66, 0x6f,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x64, 0x65, 0x76, 0x69,
	0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x64, 0x65, 0x76,
	0x69, 0x63, 0x65, 0x49, 0x64, 0x12, 0x1e, 0x0a, 0x0a, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75,
	0x72, 0x65, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x63, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x75, 0x72, 0x65, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f,
	0x74, 0x79, 0x70, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x64, 0x65, 0x76, 0x69,
	0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f,
	0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e,
	0x22, 0x5c, 0x0a, 0x16, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x44, 0x65, 0x76,
	0x69, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1f, 0x0a, 0x0b, 0x64, 0x65,
	0x76, 0x69, 0x63, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x61,
	0x70, 0x69, 0x5f, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x61, 0x70, 0x69, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x22, 0x83,
	0x01, 0x0a, 0x17, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x44, 0x65, 0x76, 0x69,
	0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x73, 0x75,
	0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x73, 0x75, 0x63,
	0x63, 0x65, 0x73, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x1b,
	0x0a, 0x09, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x49, 0x64, 0x12, 0x17, 0x0a, 0x07, 0x61,
	0x70, 0x69, 0x5f, 0x6b, 0x65, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x61, 0x70,
	0x69, 0x4b, 0x65, 0x79, 0x32, 0xc0, 0x01, 0x0a, 0x10, 0x44, 0x69, 0x73, 0x63, 0x6f, 0x76, 0x65,
	0x72, 0x79, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x52, 0x0a, 0x0d, 0x47, 0x65, 0x74,
	0x44, 0x65, 0x76, 0x69, 0x63, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x1f, 0x2e, 0x66, 0x6c, 0x65,
	0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x44, 0x65, 0x76, 0x69, 0x63, 0x65,
	0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x20, 0x2e, 0x66, 0x6c,
	0x65, 0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x44, 0x65, 0x76, 0x69, 0x63,
	0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x58, 0x0a,
	0x0f, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x44, 0x65, 0x76, 0x69, 0x63, 0x65,
	0x12, 0x21, 0x2e, 0x66, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x44, 0x65, 0x76, 0x69, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x22, 0x2e, 0x66, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x2e,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x44, 0x65, 0x76, 0x69, 0x63, 0x65, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x85, 0x01, 0x0a, 0x0d, 0x63, 0x6f, 0x6d, 0x2e,
	0x66, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x2e, 0x76, 0x31, 0x42, 0x0e, 0x44, 0x69, 0x73, 0x63, 0x6f,
	0x76, 0x65, 0x72, 0x79, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x1f, 0x66, 0x6c, 0x65,
	0x65, 0x74, 0x64, 0x2e, 0x73, 0x68, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x66, 0x6c, 0x65, 0x65, 0x74,
	0x64, 0x2f, 0x76, 0x31, 0x3b, 0x66, 0x6c, 0x65, 0x65, 0x74, 0x70, 0x62, 0xa2, 0x02, 0x03, 0x46,
	0x58, 0x58, 0xaa, 0x02, 0x09, 0x46, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x2e, 0x56, 0x31, 0xca, 0x02,
	0x09, 0x46, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x15, 0x46, 0x6c, 0x65,
	0x65, 0x74, 0x64, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61,
	0x74, 0x61, 0xea, 0x02, 0x0a, 0x46, 0x6c, 0x65, 0x65, 0x74, 0x64, 0x3a, 0x3a, 0x56, 0x31, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_fleetd_v1_discovery_proto_rawDescOnce sync.Once
	file_fleetd_v1_discovery_proto_rawDescData = file_fleetd_v1_discovery_proto_rawDesc
)

func file_fleetd_v1_discovery_proto_rawDescGZIP() []byte {
	file_fleetd_v1_discovery_proto_rawDescOnce.Do(func() {
		file_fleetd_v1_discovery_proto_rawDescData = protoimpl.X.CompressGZIP(file_fleetd_v1_discovery_proto_rawDescData)
	})
	return file_fleetd_v1_discovery_proto_rawDescData
}

var file_fleetd_v1_discovery_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_fleetd_v1_discovery_proto_goTypes = []any{
	(*GetDeviceInfoRequest)(nil),    // 0: fleetd.v1.GetDeviceInfoRequest
	(*GetDeviceInfoResponse)(nil),   // 1: fleetd.v1.GetDeviceInfoResponse
	(*ConfigureDeviceRequest)(nil),  // 2: fleetd.v1.ConfigureDeviceRequest
	(*ConfigureDeviceResponse)(nil), // 3: fleetd.v1.ConfigureDeviceResponse
}
var file_fleetd_v1_discovery_proto_depIdxs = []int32{
	0, // 0: fleetd.v1.DiscoveryService.GetDeviceInfo:input_type -> fleetd.v1.GetDeviceInfoRequest
	2, // 1: fleetd.v1.DiscoveryService.ConfigureDevice:input_type -> fleetd.v1.ConfigureDeviceRequest
	1, // 2: fleetd.v1.DiscoveryService.GetDeviceInfo:output_type -> fleetd.v1.GetDeviceInfoResponse
	3, // 3: fleetd.v1.DiscoveryService.ConfigureDevice:output_type -> fleetd.v1.ConfigureDeviceResponse
	2, // [2:4] is the sub-list for method output_type
	0, // [0:2] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_fleetd_v1_discovery_proto_init() }
func file_fleetd_v1_discovery_proto_init() {
	if File_fleetd_v1_discovery_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_fleetd_v1_discovery_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_fleetd_v1_discovery_proto_goTypes,
		DependencyIndexes: file_fleetd_v1_discovery_proto_depIdxs,
		MessageInfos:      file_fleetd_v1_discovery_proto_msgTypes,
	}.Build()
	File_fleetd_v1_discovery_proto = out.File
	file_fleetd_v1_discovery_proto_rawDesc = nil
	file_fleetd_v1_discovery_proto_goTypes = nil
	file_fleetd_v1_discovery_proto_depIdxs = nil
}
