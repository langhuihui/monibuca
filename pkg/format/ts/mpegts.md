#MPEGTS

----------

Name:苏荣
Data:2016/5/27 09:03:30


----------

## PSI(Program Specific Information) 节目特定信息
PSI 可以认为属于 6 个表:
1) 节目相关表(PAT)
2) TS 节目映射表(PMT)
3) 网络信息表(NIT)
4) 有条件访问表(CAT)
5) 传输流描述表
6) IPMP 控制信息表

##ES流(Elementary Stream):基本码流,不分段的音频、视频或其他信息的连续码流.

##PES流:把基本流ES分割成段,并加上相应头文件打包成形的打包基本码流

##PS流(Program Stream):节目流,将具有共同时间基准的一个或多个PES组合(复合)而成的单一数据流(用于播放或编辑系统,如m2p).

##TS流(Transport Stream):传输流,将具有共同时间基准或独立时间基准的一个或多个PES组合(复合)而成的单一数据流(用于数据传输).

##PES ES TS
视频压缩成H264码流,可以称之为ES流,将其每帧打包为PES流,然后分拆为多个188字节,称为TS流.

H264(ES) = PES1(一帧ES打包) + PES2(一帧ES打包) + PES3(一帧ES打包) + ...

PES1 = PES1 Header + PES1 Payload = PES1 Packet Start Code Prefix + Stream ID + PES1 Packet Length + Send PES1 Header(不确定大小) + PES1 Payload

PES1 Payload = TS1 Payload + TS2 Payload + TS3 Payload + ...

PES1 = TS1 + TS2 + TS3 + ....

PES1 = TS1(TS1 Header + PES1 Header + TS1 Payload) + TS2(有三种可能) + TS3(有三种可能) + ......

TS1(TS流第一个包) = TS1 Header + PES1 Header + TS1 Payload

TS2(TS流第二个包,第一种情况)  = TS2 Header + 自适应字段 + TS2 Payload (出现概率 1%)

TS2(TS流第二个包,第二种情况) = TS2 Header + 自适应字段 (出现概率 0.1%)

TS2(TS流第二个包,第三种情况) = TS2 Header + TS2 Payload (出现概率 98.9%)

一段ES流 = N个PES(N帧)

同一个PES的TS的PID是相同的

##寻找第一个TS包
Header PID =  0x000 说明数据包是PAT表信息
第一个TS包 一般叫做 PAT （Program Association Table,节目相关表）

TS流 : PID=005 + PID=002 + PID=000

一般来说第一个TS包一般在第一个位置,本例举出一个特殊情况(第一个TS包在第三)

在寻找第一个TS包时,不断读取TS包,直到找到pid=000的位置,并将读取过的TS包置入缓冲区

##寻找下一个TS包
第二个TS包 一般叫做PMT(Program Map Table,节目映射表)

##解析TS包
payload_unit_start_indicator : 该字段用来表示TS包的有效净荷有PES包或者PSI数据的情况.

当TS包带有PES包数据时(出现概率99.9%).不带PES包(出现概率0.1%).

1. 当TS包带有PES包数据时,payload_unit_start_indicator具有以下的特点:
a. 置为1,标识TS包的有效净荷以PES包的第一个字节开始.
b. 置为0,表示TS包的开始不是PES包.

2. 当TS包带有PSI数据时,payload_unit_start_indicator具有以下特点:
a. 置为1,表示TS包带有PSI部分的第一个字节,即第一个字节带有指针pointer_field.
b. 置为0,表示TS包不带有一个PSI部分的第一个字节,即在有效净荷中没有指针point_field.
c. 对于空包的包,payload_unit_start_indicator应该置为0

adaptionFieldControl:
01 -> 仅含有效负载(TS包第三种情况)
10 -> 仅含调整字段(TS包第二种情况)
11 -> 含有调整字段和有效负载(TS包第一种情况)

TS流,通过一个个的TS包来传送. TS包可以是传送PSI SI等各表的数据包,也可以是传送节目音视频数据(携带的PES包:音视频基本流包)的包；TS携带 PSI SI等表的数据时,各个表以各表对应的Section语法格式做为传输单元存放到TS包中 以便传输；
TS包,有一个TS包的PID,系统就是根据这个PID来找对应的TS包；对于包含音视频数据（PES包）的TS包,系统通过TS的PID找到对应TS数据包,提取其中的数据组合成节目的音视频；对于携带PSI SI等数据的TS包,系统通过TS的PID找到对应TS数据包,提取各个PSI SI数据表格,用来指导系统；因此其中部分PID用来固定传输某些数据内容.

有了TS的PID后, 如果TS包携带的是PSI SI等表格的Section数据时,有时还不能确定该PID的TS包中携带的数据是什么,SDT BAT ST 等表传送时,都用的是PID为0X0011的TS数据包,对于这种携带PSI SI Section单元的TS包,对应的数据(表的Section语法中)还有一个 TABLE_ID字段,用来可以确定是具体的什么表

因此PID+TableID就可以确定负载带了什么,是PES还是PSI.


----------


1. 第一个包:

包头 : 47 60 00 10 
0x47  : syncByte
0x6   : 0110(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 1.
0x000 : 0 0000 0000 0000, pid = 0,说明是第一个TS包(PAT表)
0x10  : 0001 0000, adaptionFieldControl = 01,说明仅含有效负载(TS包第三种情况)

负载 : 00 00 B0 0D 00 00 C1 00 00 00 01 E0
81 0C 8C BE 32 FF FF......FF

指针                : 00
table id            : 00
固定值              : B (1011)
section_length      : 0 0D(值:13)
transport_stream_id : 00 00
version number & current_next_indicator : C1
section_number      : 00
last_section_number : 00
program_number      : 00 01
program_map_PID     : E0 81(因为program_number > 0)
CRC_32              : 0C 8C BE 32

    if (program_number == 0)
    {
        network_PID
    }else
    {
        program_map_PID
    }

E0 81 = reserved3 + program_map_PID =  | 1110 0000 | 1000 0001 |
program_map_PID = 0x81(说明PMT的pid为081)


----------


2. 第二个包

包头 : 47 60 81 10
0x47  : syncByte
0x6   : 0110(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 1.
0x081 : 0 0000 1000 0001, pid = 0x081(说明是PMT表,因为前面的PAT表给出了)
0x10  : 0001 0000, adaptionFieldControl = 01,说明仅含有效负载(TS包第三种情况)

负载 : 00 02 B0 17 00 01 C1 00 00 E8 10 F0 00 1B E8 10
F0 00 03 E8 14 F0 00 66 74 A4 2D FF FF FF FF FF......FF

指针                : 00
table id            : 02
固定值              : B
section_length      : 0 17(值:23,表示到后面FF FF FF FF FF FF之前总共有23个字节)
program_number      : 00 01
reserved2 & version_number & current_next_indicator : C1
section_number      : 00
last_section_number : 00
PCR_PID             : E8 10
program_info_length : F0 00  前4位为保留位 后12位为描述信息长度 此处为0

第一流分析 : 1B E8 10 F0 00
stream_type         : 1B  视频流(H264)(ITU-T H.264建议书| SO/IEC 14496-10 视频中定义的 AVC 视频流)
elementary_PID      : E8 10  前3位为保留位取后13位  则PID=810 表示此PID的都是视频流
ES_info_length      : F0 00 前4位为保留位 后12位为描述信息长度 此处为0

第二流分析 : 03 E8 14 F0 00
stream_type         : 03  音频流(MP3)
elementary_PID      : E8 14  前3位为保留位取后13位  则PID=814 表示此PID的都是音频流
ES_info_length      : F0 00 前4位为保留位 后12位为描述信息长度 此处为0



CRC                 : 66 74 A4 2D


reserved4 + program_info_length = | 1111 0000 | 0000 0000 |
program_info_length = 0

stream_type : 03 表示流是音频流 MP3 格式   814  表示  pid=814 的TS包存储的是MP3格式的音频流.
stream_type : 01 表示流是视频流 h264格式   810  表示  pid=810 的TS包存储的是h264格式的视频流


----------


3. 第三个包
包头 : 47 48 14 10
0x47  : syncByte
0x4   : 0100(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 1.
0x814 : 0 1000 0001 0100, pid = 0x814(音频MP3)
0x10  : 0001 0000, adaptionFieldControl = 01

这里:
payload_unit_start_indicator = 1, 说明有效载荷起始符为1,含有PES包头
adaptionFieldControl = 01, 说明仅含有效负载(TS包第三种情况)

负载 : 00 00 01 C0 01 88 80 80 05 21 00 01 96 07 FF FD 85 00 33 22 22 11 22 11 11 11 11 11 11 24 82 41 00 90 40 00 00 00 00 00 40 00 ....... 70 34 5B CE 64 B7 D2 F5 4E 07 50 8E 11 1E 60 61 21 32 11 59

packetStartCodePrefix   : 00 00 01 
streamID                : C0
pes_PacketLength        : 01 88(值为392,占用392个字节,一帧数据长度,也可以置为0)
Sned PES HEADER         : 占用不确定位 本例为:80 80 05 21 00 01 96 07


Sned PES HEADER 包括以下几个字段: 80 80 05 21 00 01 96 07(解析为二进制显示)
| 8    0   | 8    0    | 0    5    | 2    1    | 0    0    | 0    1    | 9    6    | 0    7    |
| 1000 0000| 1000 0000 | 0000 0101 | 0010 0001 | 0000 0000 | 0000 0001 | 1001 0110 | 0000 1110 |

(注意,下面的数值是用二进制表示,不特别声明,都是用16进制表示)
(0x80)
constTen                    : 10 固定
PES_scrambling_control      : 00 PES加扰控制
PES_priority                : 0 PES 包中该有效载荷的优先级
data_alignment_indicator    : 0 数据定位指示符
copyright                   : 0 PES 包有效载荷的素材依靠版权所保护
original_or_copy            : 0 PES 包有效载荷的内容是原始的

(0x80)
PTS_DTS_flags               : 10 PES 包头中 PTS 字段存在
ESCR_flag                   : 0
ES_rate_flag                : 0
DSM_trick_mode_flag         : 0
additional_copy_info_flag   : 0
PES_CRC_flag                : 0
PES_extension_flag          : 0

(0x05)
PES_header_data_length      : 0000 0101(值为5)PES头数据长度,表示后面还有5个字节,之后就是一帧的数据

(0x4200032C)(十进制:1107297068)
PTS(presentation time stamp): 0010 0001 0000 0000 0000 0001 1001 0110 0 

下面字段在本例中都没有:
ESCR(42) = ESCR_base(33) + ESCR_extension(9)
ES_rate(22)
DSM特技方式(8)
additional_copy_info(7)
previous_PES_packet_CRC(16)
PES_Extension(不确定)


因为 PTS_DTS_flags == 10,所以本例中只有PTS没有DTS.


注意 : 本TS包 包含PES头信息 说明开始下一帧

----------


4. 第四个包
包头 : 47 08 14 11
0x47  : syncByte
0x0   : 0000(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 0.
0x814 : 0 1000 0001 0100, pid = 0x814(音频MP3)
0x11  : 0001 0001, adaptionFieldControl = 01

这里:
payload_unit_start_indicator = 0, 说明有效载荷起始符为0,不含有PES包头
adaptionFieldControl = 01, 说明仅含有效负载(TS包第三种情况)

----------


5. 第五个包
包头 : 47 08 14 32
0x47  : syncByte
0x0   : 0000(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 0.
0x814 : 0 1000 0001 0100, pid = 0x814(音频MP3)
0x32  : 0011 0010, adaptionFieldControl = 11

这里:
payload_unit_start_indicator = 0, 说明有效载荷起始符为0,不含有PES包头
adaptionFieldControl = 11, 说明先有自适应字段,再有有效载荷(TS包第一种情况)

负载 : 99 00 FF FF FF ... FF 52 DE E6 B5 D0 76 CD CB B2 24 B3 92 AD 4E CD 19 D2 CC 82 D4 78 10 80 6C 0E 99 49 A4 59 C0

adaptation_field_length : 99(值为153,表示占用153个字节)

discontinuity_indicator & random_access_indicator & 
elementary_stream_priority_indicator & PCR_flag & 
OPCR_flag & splicing_point_flag & 
transport_private_data_flag & adaptation_field_extension_flag : 00 剩下的所有字段都为0

(00 FF FF FF ... FF)这里都是调整字段,从52 DE E6 B5 D0(从00(FF之前,99之后) 开始算是第1个字节,跳到第153个字节)开始,就是真正的帧数据了


----------


6. 第六个包
包头 : 47 48 14 13
0x47  : syncByte
0x4   : 0100(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 1.
0x814 : 0 1000 0001 0100, pid = 0x814(音频MP3)
0x13  : 0001 0011, adaptionFieldControl = 01,说明仅含有效负载(TS包第三种情况)

这里:
payload_unit_start_indicator = 1, 说明有效载荷起始符为1,含有PES包头
adaptionFieldControl = 01, 说明仅含有效负载(TS包第三种情况)

负载 : 00 00 01 C0 01 88 80 80 05 21 00 01 A6 E7 FF FD

packetStartCodePrefix   : 00 00 01 
streamID                : C0
pes_PacketLength        : 01 88(值为392,占用392个字节)
Sned PES HEADER         : 占用不确定位

所以本包数据流ID 和 第二个包的流ID是一样的

注意 : 本TS包 又包含PES头信息 说明开始下一帧


----------

7. 第七个包
包头 : 47 48 10 30
0x47  : syncByte
0x4   : 0100(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 1.
0x810 : 0 1000 0001 0000, pid = 0x810(视频H264)
0x30  : 0011 0000, adaptionFieldControl = 11,说明含有调整字段和有效负载(TS包第一种情况)

这里:
payload_unit_start_indicator = 1, 说明有效载荷起始符为1,含有PES包头
adaptionFieldControl = 11, 说明含有调整字段和有效负载(TS包第一种情况)

负载 : 07 10 00 00 01 0F 7E 88 00 00 01 E0 00 00 80 C0 0A 31 00 01 96 07 11 00 01 7E 91 00 00 00 01 67 4D 40 1E 96 ...... D2 99 71 F3

adaptation_field_length : 07(值为7,表示占用153个字节)

discontinuity_indicator & random_access_indicator & 
elementary_stream_priority_indicator & PCR_flag & 
OPCR_flag & splicing_point_flag & 
transport_private_data_flag & adaptation_field_extension_flag : 10

(10 00 00 01 0F 7E 88)调整字段

packetStartCodePrefix   : 00 00 01 
streamID                : EO
pes_PacketLength        : 00 00(值为0,占用0个字节,一帧数据长度,也可以置为0,此时需要自己去计算)
Sned PES HEADER         : 占用不确定位


----------


8. 第八个包 
包头 : 47 08 10 11
0x47  : syncByte
0x0   : 0000(这里的最后一个字节,要给到下面),payload_unit_start_indicator = 0.
0x810 : 0 1000 0001 0000, pid = 0x810(视频H264)
0x11  : 0001 0001, adaptionFieldControl = 01, 说明仅含有效负载(TS包第三种情况)

这里:
payload_unit_start_indicator = 0, 说明有效载荷起始符为0,不含有PES包头
adaptionFieldControl = 01, 说明仅含有效负载(TS包第三种情况)


----------

总结这个八个包:

第一个TS包(PID:0X00)  : 包含了PAT.
第二个TS包(PID:0X81)  : 包含了PMT.
第三个TS包(PID:0x814) : 音频PES包头所有的TS包.
第四个TS包(PID:0x814) : 音频TS包.
第五个TS包(PID:0x814) : 音频TS包.
第六个TS包(PID:0x814) : 音频PES包头所有的TS包.
第七个TS包(PID:0x810) : 视频PES包头所有的TS包.
第八个TS包(PID:0x810) : 视频TS包.


----------


// Packet Header:
// PID是TS流中唯一识别标志,Packet Data是什么内容就是由PID决定的.如果一个TS流中的一个Packet的Packet Header中的PID是0x0000,
// 那么这个Packet的Packet Data就是DVB的PAT表而非其他类型数据(如Video,Audio或其他业务信息).

// 分析一个Header:
// 二进制:   0100 0111 0000 0111 1110 0101 0001 0010
// 十六进制: 4    7    0    7    e    5    1    2

// syncByte = 0x47          就是0x47,这是DVB TS规定的同步字节,固定是0x47
// transportErrorIndicator = 0    表示当前包没有发生传输错误
// payloadUnitStartIndicator = 0  具体含义参考ISO13818-1标准文档
// transportPriority = 0      表示当前包是低优先级
// pid = 0x07e5(0 0111 1110 0101) Video PID
// transportScramblingControl = 00  表示节目没有加密
// adaptionFieldControl = 01    具体含义参考ISO13818-1标准文档
// continuityCounter = 0010     表示当前传送的相同类型的包是第3个


----------


// 分析一段TS流:(PAT)
// Packet Header : 0x47 0x40 0x00 0x10
// Packet Data   : 00 00 b0 11 00 01 c1 00 00 00 00 e0 1f 00 01 e1 00 24 ac48 84 ff ff ... ff ff

// Header PID = 0x0000 说明数据包是PAT表信息,包头后需要除去一个字节才是有效数据(payload_unit_start_indicator="1")
// 所以,Packet Data就应该是 : 00 b0 11 00 01 c1 00 00 00 00 e0 1f 00 01 e1 00 24 ac48 84 ff ff ... ff ff

//
// 00 | b0 11 | 00 01 | c1 | 00 | 00 | 00 00 | e0 1f | 00 01 e1 00 |
//

// table_id = 0000 0000

// section_syntax_indicator = 1
// zero = 0
// reserved1 = 11
// sectionLength = 0000 0001 0001

// transportStreamID = 0000 0000 0000 0001

// reserved2 = 11
// versionNumber = 0000 0
// currentNextIndicator 1

// sectionNumber = 0000 0000

// lastSectionNumber = 0000 0000

// programNumber = 0000 0000 0000 0000

// reserved3 = 111
// networkPID = 0 0000 0001 1111

// crc32


----------


// 分析一段TS流:(PMT)
// Packet Header : 0x47 0x43 0xe8 0x12
// Packet Data   : 00 02 b0 12 00 01 c1 00 00 e3 e9 f0 00 1b e3 e9 f0 00 f0 af b4 4f ff ff ... ff ff

// Header PID = 0x03e8 说明数据包是PMT表信息,包头后需要除去一个字节才是有效数据(payload_unit_start_indicator="1")
// 所以,Packet Data就应该是 : 02 b0 12 00 01 c1 00 00 e3 e9 f0 00 1b e3 e9 f0 00 f0 af b4 4f ff ff ... ff ff

// 1    2       3       4    5    6    7       8       9    10      11      12
// 02 | b0 12 | 00 01 | c1 | 00 | 00 | e3 e9 | f0 00 | 1b | e3 e9 | f0 00 | f0 af b4 4f |
//

// 1:
// table_id = 0000 0010

// 2:
// section_syntax_indicator = 1
// zero = 0
// reserved1 = 11
// section_length = 0000 0001 0010

// 3:
// program_number = 0000 0000 0000 0001

// 4:
// reserved2 = 11
// version_number = 00 000
// current_next_indicator = 1

// 5:
// section_number = 0000 0000

// 6:
// last_section_number = 0000 0000

// 7:
// reserved3 = 111
// PCR_PID = 0 0011 1110 1001

// 8:
// reserved4 = 1111
// program_info_length = 0000 0000 0000

// 9:
// stream_type = 0001 1011

// 10:
// reserved5 = 111
// elementary_PID = 0 0011 1110 1001

// 11:
// reserved6 = 1111
// ES_info_length = 0000 0000 0000

// 12:
// crc


----------


##TS流解码过程
1. 获取TS中的PAT
2. 获取TS中的PMT
3. 根据PMT可以知道当前网络中传输的视频（音频）类型（H264）,相应的PID,PCR的PID等信息.
4. 设置demux 模块的视频Filter 为相应视频的PID和stream type等.
5. 从视频Demux Filter 后得到的TS数据包中的payload 数据就是 one piece of PES,在TS header中有一些关于此 payload属于哪个 PES的 第多少个数据包. 因此软件中应该将此payload中的数据copy到PES的buffer中,用于拼接一个PES包.
6. 拼接好的PES包的包头会有 PTS,DTS信息,去掉PES的header就是 ES.
7. 直接将 被被拔掉 PES包头的ES包送给decoder就可以进行解码.解码出来的数据就是一帧一帧的视频数据,这些数据至少应当与PES中的PTS关联一下,以便进行视音频同步.
8. I,B,B,P 信息是在ES中的.


----------


1. 首先找到PID为0x00的TS包,找到里面的节目映射表（PMT)PID,因为可能有几个节目信息.所以可能有几个PMT_PID,以一个为例
2. 接着查找该PMT_PID的TS包,通常就紧接着.在该PMT包中找音频和视频的PID.以视频为例.
3. 开始提取一帧ES数据
  3.1  查找视频PID的TS包
  3.2  找PES包头,方法:TS包头第2个字节的高6位（有效载荷单元起始指示符）为1的TS包,跳过自适应字段,找到PES包头,提取时间戳,再跳至ES数据,这就是一帧ES数据的开始部分.
  3.3  查找有效载荷单元起始指示符为0的TS包.跳过TS包头,跳过自适应字段,提取后面的ES数据
  3.4  同3.3接着查找
  3.5  当碰到有效载荷单元起始指示符又变为1的视频TS包,就知道这是下一帧的开始了,将前面的所有ES数据组合成一帧数据.开始下一轮组帧.


----------


##参考文档:

1. [TS流](http://blog.csdn.net/cabbage2008/article/category/5885203)
1. [TS各个表 与 SECTION 的解析 CAS原理 ](http://blog.sina.com.cn/s/blog_6b94d5680101r5l6.html)