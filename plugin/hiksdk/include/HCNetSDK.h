#ifndef _HC_NET_SDK_H_
#define _HC_NET_SDK_H_

#ifndef _WINDOWS_
    #if (defined(_WIN32) || defined(_WIN64))
        #include <winsock2.h>
        #include <windows.h>
    #endif
#endif

#ifndef __PLAYRECT_defined
    #define __PLAYRECT_defined
    typedef struct __PLAYRECT
    {
        int x;
        int y;
        int uWidth;
        int uHeight;
    }PLAYRECT;
#endif

#if (defined(_WIN32)) //windows
    typedef  unsigned __int64   UINT64;
    typedef  signed   __int64   INT64;
#elif defined(__linux__) || defined(__APPLE__) //linux
    #define  BOOL  int
      #include <stdint.h>
      typedef uint32_t    DWORD;
      typedef uint16_t    WORD;
      typedef uint16_t    SHORT;
      typedef uint16_t    USHORT;
      typedef int32_t     LONG;
      typedef uint8_t     BYTE;
      typedef uint32_t    UINT;
      typedef void*       LPVOID;
      typedef void*       HANDLE;
      typedef uint32_t *  LPDWORD;
      typedef uint64_t    UINT64;

    #ifndef TRUE
        #define TRUE  1
    #endif
    #ifndef FALSE
        #define FALSE 0
    #endif
    #ifndef NULL
        #define NULL 0
    #endif

    #define __stdcall
    #define CALLBACK

    #define NET_DVR_API extern "C"
    typedef unsigned int   COLORKEY;
    typedef unsigned int   COLORREF;

    #ifndef __HWND_defined
        #define __HWND_defined
        #if defined(__linux__)
            typedef unsigned int HWND;
        #else
            typedef void* HWND;
        #endif
    #endif

    #ifndef __HDC_defined
        #define __HDC_defined
        #if defined(__linux__)
            typedef struct __DC
            {
                void*   surface;        //SDL Surface
                HWND    hWnd;           //HDC window handle
            }DC;
            typedef DC* HDC;
        #else
            typedef void* HDC;
        #endif
    #endif

    typedef struct tagInitInfo
    {
        int uWidth;
        int uHeight;
    }INITINFO;
#endif

#define SERIALNO_LEN            48      //序列号长度
#define NET_DVR_DEV_ADDRESS_MAX_LEN 129
#define NET_DVR_LOGIN_USERNAME_MAX_LEN 64
#define NET_DVR_LOGIN_PASSWD_MAX_LEN 64


#define LIGHT_PWRON        2    /* 接通灯光电源 */
#define WIPER_PWRON        3    /* 接通雨刷开关 */
#define FAN_PWRON        4    /* 接通风扇开关 */
#define HEATER_PWRON    5    /* 接通加热器开关 */
#define AUX_PWRON1        6    /* 接通辅助设备开关 */
#define AUX_PWRON2        7    /* 接通辅助设备开关 */
#define SET_PRESET        8    /* 设置预置点 */
#define CLE_PRESET        9    /* 清除预置点 */

#define ZOOM_IN            11    /* 焦距以速度SS变大(倍率变大) */
#define ZOOM_OUT        12    /* 焦距以速度SS变小(倍率变小) */
#define FOCUS_NEAR      13  /* 焦点以速度SS前调 */
#define FOCUS_FAR       14  /* 焦点以速度SS后调 */
#define IRIS_OPEN       15  /* 光圈以速度SS扩大 */
#define IRIS_CLOSE      16  /* 光圈以速度SS缩小 */

#define TILT_UP            21    /* 云台以SS的速度上仰 */
#define TILT_DOWN        22    /* 云台以SS的速度下俯 */
#define PAN_LEFT        23    /* 云台以SS的速度左转 */
#define PAN_RIGHT        24    /* 云台以SS的速度右转 */
#define UP_LEFT            25    /* 云台以SS的速度上仰和左转 */
#define UP_RIGHT        26    /* 云台以SS的速度上仰和右转 */
#define DOWN_LEFT        27    /* 云台以SS的速度下俯和左转 */
#define DOWN_RIGHT        28    /* 云台以SS的速度下俯和右转 */
#define PAN_AUTO        29    /* 云台以SS的速度左右自动扫描 */

#define FILL_PRE_SEQ    30    /* 将预置点加入巡航序列 */
#define SET_SEQ_DWELL    31    /* 设置巡航点停顿时间 */
#define SET_SEQ_SPEED    32    /* 设置巡航速度 */
#define CLE_PRE_SEQ        33    /* 将预置点从巡航序列中删除 */
#define STA_MEM_CRUISE    34    /* 开始记录轨迹 */
#define STO_MEM_CRUISE    35    /* 停止记录轨迹 */
#define RUN_CRUISE        36    /* 开始轨迹 */
#define RUN_SEQ            37    /* 开始巡航 */
#define STOP_SEQ        38    /* 停止巡航 */
#define GOTO_PRESET        39    /* 快球转到预置点 */

#define DEL_SEQ         43  /* 删除巡航路径 */
#define STOP_CRUISE        44    /* 停止轨迹 */
#define DELETE_CRUISE    45    /* 删除单条轨迹 */
#define DELETE_ALL_CRUISE 46/* 删除所有轨迹 */

#define PAN_CIRCLE      50   /* 云台以SS的速度自动圆周扫描 */
#define DRAG_PTZ        51   /* 拖动PTZ */
#define LINEAR_SCAN     52   /* 区域扫描 */ //2014-03-15
#define CLE_ALL_PRESET  53   /* 预置点全部清除 */
#define CLE_ALL_SEQ     54   /* 巡航全部清除 */
#define CLE_ALL_CRUISE  55   /* 轨迹全部清除 */

#define POPUP_MENU      56   /* 显示操作菜单 */

#define TILT_DOWN_ZOOM_IN    58    /* 云台以SS的速度下俯&&焦距以速度SS变大(倍率变大) */
#define TILT_DOWN_ZOOM_OUT  59  /* 云台以SS的速度下俯&&焦距以速度SS变小(倍率变小) */
#define PAN_LEFT_ZOOM_IN    60  /* 云台以SS的速度左转&&焦距以速度SS变大(倍率变大)*/
#define PAN_LEFT_ZOOM_OUT   61  /* 云台以SS的速度左转&&焦距以速度SS变小(倍率变小)*/
#define PAN_RIGHT_ZOOM_IN    62  /* 云台以SS的速度右转&&焦距以速度SS变大(倍率变大) */
#define PAN_RIGHT_ZOOM_OUT  63  /* 云台以SS的速度右转&&焦距以速度SS变小(倍率变小) */
#define UP_LEFT_ZOOM_IN     64  /* 云台以SS的速度上仰和左转&&焦距以速度SS变大(倍率变大)*/
#define UP_LEFT_ZOOM_OUT    65  /* 云台以SS的速度上仰和左转&&焦距以速度SS变小(倍率变小)*/
#define UP_RIGHT_ZOOM_IN    66  /* 云台以SS的速度上仰和右转&&焦距以速度SS变大(倍率变大)*/
#define UP_RIGHT_ZOOM_OUT   67  /* 云台以SS的速度上仰和右转&&焦距以速度SS变小(倍率变小)*/
#define DOWN_LEFT_ZOOM_IN   68  /* 云台以SS的速度下俯和左转&&焦距以速度SS变大(倍率变大) */
#define DOWN_LEFT_ZOOM_OUT  69  /* 云台以SS的速度下俯和左转&&焦距以速度SS变小(倍率变小) */
#define DOWN_RIGHT_ZOOM_IN    70  /* 云台以SS的速度下俯和右转&&焦距以速度SS变大(倍率变大) */
#define DOWN_RIGHT_ZOOM_OUT    71  /* 云台以SS的速度下俯和右转&&焦距以速度SS变小(倍率变小) */
#define TILT_UP_ZOOM_IN        72    /* 云台以SS的速度上仰&&焦距以速度SS变大(倍率变大) */
#define TILT_UP_ZOOM_OUT    73

//宏定义 修正后
#define MAX_NAMELEN                16        //DVR本地登陆名
#define MAX_RIGHT                32        //设备支持的权限（1-12表示本地权限，13-32表示远程权限）
#define NAME_LEN                32      //用户名长度
#define MIN_PASSWD_LEN          8          //最小密码长度
#define PASSWD_LEN                16      //密码长度
#define STREAM_PASSWD_LEN         12      //码流加密密钥最大长度
#define MAX_PASSWD_LEN_EX            64      //密码长度64位
#define GUID_LEN                16      //GUID长度
#define DEV_TYPE_NAME_LEN        24      //设备类型名称长度
#define SERIALNO_LEN            48      //序列号长度
#define MACADDR_LEN                6       //mac地址长度
#define MAC_ADDRESS_NUM         48      //Mac地址长度
#define MAX_SENCE_NUM           16      //场景数
#define RULE_REGION_MAX         128      //最大区域
#define MAX_ETHERNET            2       //设备可配以太网络
#define MAX_NETWORK_CARD        4       //设备可配最大网卡数目
#define MAX_NETWORK_CARD_EX     12      //设备可配最大网卡数目扩展
#define PATHNAME_LEN            128     //路径长度
#define MAX_PRESET_V13          16      //预置点
#define MAX_TEST_COMMAND_NUM   32      //产线测试保留字段长度
#define MAX_NUMBER_LEN            32        //号码最大长度
#define MAX_NAME_LEN            128        //设备名称最大长度
#define MAX_INDEX_LED           8       //LED索引最大值 2013-11-19
#define    MAX_CUSTOM_DIR            64      //自定义目录最大长度
#define URL_LEN_V40             256        //最大URL长度
#define CLOUD_NAME_LEN          48      //云存储服务器用户名长度
#define CLOUD_PASSWD_LEN        48      //云存储服务器密码长度
#define MAX_SENSORNAME_LEN      64      //传感器名称长度
#define MAX_SENSORCHAN_LEN      32      //传感器通道长度
#define MAX_DESCRIPTION_LEN     32      //传感器描述长度
#define MAX_DEVNAME_LEN_EX      64      //设备名称长度扩展
#define NET_SDK_MAX_FILE_PATH   256     //文件路径长度 
#define MAX_TMEVOICE_LEN        64      //TME语音播报内容长度
#define ISO_8601_LEN            32      //ISO_8601时间长度
#define MODULE_INFO_LEN            32    //模块信息长度
#define VERSION_INFO_LEN        32    //版本信息长度

#define MAX_NUM_INPUT_BOARD     512     //输入板最大个数
#define MAX_SHIPSDETE_REGION_NUM    8 // 船只检测区域列表最大数目

#define MAX_RES_NUM_ONE_VS_INPUT_CHAN  8  //一个虚拟屏输入通道支持的分辨率的最大数量
#define MAX_VS_INPUT_CHAN_NUM  16  //虚拟屏输入通道最大数量

#define NET_SDK_MAX_FDID_LEN 256//人脸库ID最大长度
#define NET_SDK_MAX_PICID_LEN 256 //人脸ID最大长度
#define NET_SDK_FDPIC_CUSTOM_INFO_LEN 96 //人脸库图片自定义信息长度
#define NET_DVR_MAX_FACE_ANALYSIS_NUM      32   //最大支持单张图片识别出的人脸区域个数
#define NET_DVR_MAX_FACE_SEARCH_NUM      5   //最大支持搜索人脸区域个数
#define NET_SDK_SECRETKEY_LEN      128   //配置文件密钥长度
#define NET_SDK_CUSTOM_LEN                  512 //自定义信息最大长度
#define NET_SDK_CHECK_CODE_LEN          128//校验码长度
#define RELATIVE_CHANNEL_LEN        2//报警关联的通道号的数量
#define NET_SDK_MAX_CALLEDTARGET_NAME 32 //呗呼叫目标的用户名
#define NET_SDK_MAX_HBDID_LEN 256 /*256 人体库ID最大长度*/
//小间距LED控制器
#define  MAX_LEN_TEXT_CONTENT    128  //字符内容长度
#define  MAX_NUM_INPUT_SOURCE_TEXT    32    //信号源可叠加的文本数量
#define  MAX_NUM_OUTPUT_CHANNEL  512  //LED区域包含的输出口个数

//子窗口解码OSD
#define MAX_LEN_OSD_CONTENT  256  //OSD信息最大长度
#define MAX_NUM_OSD_ONE_SUBWND  8  //单个子窗口支持的最大OSD数量
#define MAX_NUM_SPLIT_WND  64 //单个窗口支持的最大分屏窗口数量（即子窗口数量）
#define MAX_NUM_OSD 8

//2013-11-19
#define MAX_DEVNAME_LEN         32      //设备名称最大长度
#define MAX_LED_INFO            256     //屏幕字体显示信息最大长度
#define MAX_TIME_LEN            32      //时间最大长度
#define MAX_CARD_LEN            24      //卡号最大长度
#define MAX_OPERATORNAME_LEN    32      //操作人员名称最大长度

#define THERMOMETRY_ALARMRULE_NUM 40     //热成像报警规则数
#define MAX_THERMOMETRY_REGION_NUM  40  //热度图检测区域最大支持数
#define MAX_THERMOMETRY_DIFFCOMPARISON_NUM  40 //热成像温差报警规则数
#define MAX_SHIPS_NUM           20      //船只检测最大船只数
#define MAX_SHIPIMAGE_NUM       6       //船只最大抓图数
#define KEY_WORD_NUM             3 //关键字个数
#define KEY_WORD_LEN            128  //关键字长度
//异步登录回调状态宏定义
#define ASYN_LOGIN_SUCC            1        //异步登录成功
#define ASYN_LOGIN_FAILED        0        //异步登录失败

#define NET_SDK_MAX_VERIFICATION_CODE_LEN  32        //萤石云验证码长度
#define NET_SDK_MAX_OPERATE_CODE_LEN  64        //萤石云操作码长度
#define MAX_TIMESEGMENT_V30        8       //9000设备最大时间段数
#define MAX_TIMESEGMENT            4       //8000设备最大时间段数
#define MAX_ICR_NUM             8       //抓拍机红外滤光片预置点数2013-07-09
#define MAX_VEHICLEFLOW_INFO                       24       //车流量信息最大个数
#define MAX_SHELTERNUM            4       //8000设备最大遮挡区域数
#define MAX_DAYS                7       //每周天数
#define PHONENUMBER_LEN            32      //pppoe拨号号码最大长度
#define MAX_ACCESSORY_CARD      256      //配件板信息最大长度
#define MAX_DISKNUM_V30            33        //9000设备最大硬盘数/* 最多33个硬盘(包括16个内置SATA硬盘、1个eSATA硬盘和16个NFS盘) */
#define NET_SDK_MAX_NET_USER_NUM        64    //网络用户

#define NET_SDK_DISK_LOCATION_LEN  16      //硬盘位置长度
#define NET_SDK_SUPPLIER_NAME_LEN  32      //供应商名称长度
#define NET_SDK_DISK_MODEL_LEN     64      //硬盘型号长度
#define NET_SDK_MAX_DISK_VOLUME    33      //最大硬盘卷个数
#define NET_SDK_DISK_VOLUME_LEN    36      //硬盘卷名称长度

#define MAX_DISKNUM                16      //8000设备最大硬盘数
#define MAX_DISKNUM_V10            8       //1.2版本之前版本
#define CARD_READER_DESCRIPTION    32            //读卡器描述
#define MAX_FACE_NUM               2             //最大人脸数

#define MAX_WINDOW_V30            32      //9000设备本地显示最大播放窗口数
#define MAX_WINDOW_V40            64      //Netra 2.3.1扩展
#define MAX_WINDOW                16      //8000设备最大硬盘数
#define MAX_VGA_V30                4       //9000设备最大可接VGA数
#define MAX_VGA                    1       //8000设备最大可接VGA数

#define MAX_USERNUM_V30            32      //9000设备最大用户数
#define MAX_USERNUM                16      //8000设备最大用户数
#define MAX_EXCEPTIONNUM_V30    32      //9000设备最大异常处理数
#define MAX_EXCEPTIONNUM        16      //8000设备最大异常处理数
#define MAX_LINK                6       //8000设备单通道最大视频流连接数
#define MAX_ITC_EXCEPTIONOUT    32      //抓拍机最大报警输出
#define MAX_SCREEN_DISPLAY_LEN            512    //屏幕显示字符长度

#define MAX_DECPOOLNUM            4       //单路解码器每个解码通道最大可循环解码数
#define MAX_DECNUM                4       //单路解码器的最大解码通道数（实际只有一个，其他三个保留）
#define MAX_TRANSPARENTNUM        2       //单路解码器可配置最大透明通道数
#define MAX_CYCLE_CHAN            16      //单路解码器最大轮巡通道数
#define MAX_CYCLE_CHAN_V30      64      //最大轮巡通道数（扩展）
#define MAX_DIRNAME_LENGTH        80      //最大目录长度
#define MAX_WINDOWS                16      //最大窗口数


#define MAX_STRINGNUM_V30        8        //9000设备最大OSD字符行数数
#define MAX_STRINGNUM            4       //8000设备最大OSD字符行数数
#define MAX_STRINGNUM_EX        8       //8000定制扩展
#define MAX_AUXOUT_V30            16      //9000设备最大辅助输出数
#define MAX_AUXOUT                4       //8000设备最大辅助输出数
#define MAX_HD_GROUP            16      //9000设备最大硬盘组数
#define MAX_HD_GROUP_V40        32      //设备最大硬盘组数
#define MAX_NFS_DISK            8       //8000设备最大NFS硬盘数
#define NET_SDK_VERSION_LIST_LEN 64 //算法库版本最大值
#define IW_ESSID_MAX_SIZE        32      //WIFI的SSID号长度
#define IW_ENCODING_TOKEN_MAX    32      //WIFI密锁最大字节数
#define MAX_SERIAL_NUM            64        //最多支持的透明通道路数
#define MAX_DDNS_NUMS            10      //9000设备最大可配ddns数
#define MAX_DOMAIN_NAME            64        /* 最大域名长度 */
#define MAX_EMAIL_ADDR_LEN        48      //最大email地址长度
#define MAX_EMAIL_PWD_LEN        32      //最大email密码长度
#define MAX_SLAVECAMERA_NUM     8       //从摄像机个数
#define MAX_CALIB_NUM           6       //标定点的个数
#define MAX_CALIB_NUM_EX        20      //扩展标定点的个数   
#define MAX_LEDDISPLAYINFO_LEN  1024    //最大LED屏显示长度
#define MAX_PEOPLE_DETECTION_NUM    8  //最大人员检测区域数
#define MAXPROGRESS                100     //回放时的最大百分率
#define MAX_SERIALNUM            2       //8000设备支持的串口数 1-232， 2-485
#define CARDNUM_LEN                20      //卡号长度
#define PATIENTID_LEN              64
#define CARDNUM_LEN_OUT            32      //外部结构体卡号长度
#define MAX_VIDEOOUT_V30        4       //9000设备的视频输出数
#define MAX_VIDEOOUT            2       //8000设备的视频输出数

#define MAX_PRESET_V30            256        /* 9000设备支持的云台预置点数 */
#define MAX_TRACK_V30            256        /* 9000设备支持的云台数 */
#define MAX_CRUISE_V30            256        /* 9000设备支持的云台巡航数 */
#define MAX_PRESET                128        /* 8000设备支持的云台预置点数 */
#define MAX_TRACK                128        /* 8000设备支持的云台数 */
#define MAX_CRUISE                128        /* 8000设备支持的云台巡航数 */

#define MAX_PRESET_V40            300        /* 云台支持的最大预置点数 */
#define MAX_CRUISE_POINT_NUM    128     /* 最大支持的巡航点的个数 */
#define MAX_CRUISEPOINT_NUM_V50 256     //最大支持的巡航点的个数扩展

#define CRUISE_MAX_PRESET_NUMS    32         /* 一条巡航最多的巡航点 */
#define MAX_FACE_PIC_NUM        30      /*人脸子图个数*/
#define LOCKGATE_TIME_NUM       4       //锁闸时间段个数

#define MAX_SERIAL_PORT         8       //9000设备支持232串口数
#define MAX_PREVIEW_MODE        8       /* 设备支持最大预览模式数目 1画面,4画面,9画面,16画面.... */
#define MAX_MATRIXOUT           16      /* 最大模拟矩阵输出个数 */
#define LOG_INFO_LEN            11840   /* 日志附加信息 */
#define DESC_LEN                16      /* 云台描述字符串长度 */
#define PTZ_PROTOCOL_NUM        200     /* 9000最大支持的云台协议数 */
#define IPC_PROTOCOL_NUM        50   //ipc 协议最大个数

#define MAX_AUDIO                1       //8000语音对讲通道数
#define MAX_AUDIO_V30            2       //9000语音对讲通道数
#define MAX_CHANNUM                16      //8000设备最大通道数
#define MAX_ALARMIN                16      //8000设备最大报警输入数
#define MAX_ALARMOUT            4       //8000设备最大报警输出数
#define MAX_AUDIOCAST_CFG_TYPE  3       //支持广播参数配置的类型数量 MP3、MPEG2、AAC
//9000 IPC接入
#define MAX_ANALOG_CHANNUM      32      //最大32个模拟通道
#define MAX_ANALOG_ALARMOUT     32      //最大32路模拟报警输出 
#define MAX_ANALOG_ALARMIN      32      //最大32路模拟报警输入

#define MAX_IP_DEVICE           32      //允许接入的最大IP设备数
#define MAX_IP_DEVICE_V40       64      // 允许接入的最大IP设备数 最多可添加64个 IVMS 2000等新设备
#define MAX_IP_CHANNEL          32      //允许加入的最多IP通道数
#define MAX_IP_ALARMIN          128     //允许加入的最多报警输入数
#define MAX_IP_ALARMOUT         64      //允许加入的最多报警输出数
#define MAX_IP_ALARMIN_V40      4096    //允许加入的最多报警输入数
#define MAX_IP_ALARMOUT_V40     4096    //允许加入的最多报警输出数

#define MAX_RECORD_FILE_NUM     20      // 每次删除或者刻录的最大文件数
//SDK_V31 ATM
#define MAX_ACTION_TYPE            12        //自定义协议叠加交易行为最大行为个数 
#define MAX_ATM_PROTOCOL_NUM    256   //每种输入方式对应的ATM最大协议数
#define ATM_CUSTOM_PROTO        1025   //自定义协议 值为1025
#define ATM_PROTOCOL_SORT       4       //ATM协议段数 
#define ATM_DESC_LEN            32      //ATM描述字符串长度
// SDK_V31 ATM


#define MAX_IPV6_LEN              64   //IPv6地址最大长度
#define MAX_EVENTID_LEN         64   //事件ID长度

#define INVALID_VALUE_UINT32    0xffffffff   //无效值
#define MAX_CHANNUM_V40         512
#define MAX_MULTI_AREA_NUM      24

//SDK 录播主机
#define COURSE_NAME_LEN                32    //课程名称
#define INSTRUCTOR_NAME_LEN            16    //授课教师
#define COURSE_DESCRIPTION_LEN        256    //课程信息

#define MAX_TIMESEGMENT_V40            16    //每节课信息


#define MAX_MIX_CHAN_NUM        16    /*目前支持的最大混音通道数，背景通道 + MIC + LINE IN + 最多4个小画面*/ 
#define MAX_LINE_IN_CHAN_NUM    16    //最大line in通道数
#define MAX_MIC_CHAN_NUM        16    //最大MIC通道数
#define INQUEST_CASE_NO_LEN        64    //审讯案件编号长度
#define INQUEST_CASE_NAME_LEN    64    //审讯案件名称长度
#define CUSTOM_INFO_LEN            64    //自定义信息长度
#define INQUEST_CASE_LEN        64    //审讯信息长度


#define MAX_FILE_ID_LEN         128    //视图库项目中文件ID的最大长度
#define MAX_PIC_NAME_LEN        128 //图片名称长度

/* 最大支持的通道数 最大模拟加上最大IP支持 */
#define MAX_CHANNUM_V30               ( MAX_ANALOG_CHANNUM + MAX_IP_CHANNEL )//64
#define MAX_ALARMOUT_V40             (MAX_IP_ALARMOUT_V40 +MAX_ANALOG_ALARMOUT) //4128
#define MAX_ALARMOUT_V30              ( MAX_ANALOG_ALARMOUT + MAX_IP_ALARMOUT )//96
#define MAX_ALARMIN_V30               ( MAX_ANALOG_ALARMIN + MAX_IP_ALARMIN )//160
#define MAX_ALARMIN_V40             (MAX_IP_ALARMIN_V40 +MAX_ANALOG_ALARMOUT) //4128
#define MAX_ANALOG_ALARM_WITH_VOLT_LIMIT    16 //受电压限定的模拟报警最大输入数

#define MAX_ROIDETECT_NUM       8    //支持的ROI区域数
#define MAX_LANERECT_NUM        5    //最大车牌识别区域数
#define MAX_FORTIFY_NUM         10   //最大布防个数
#define MAX_INTERVAL_NUM        4    //最大时间间隔个数
#define MAX_CHJC_NUM            3    //最大车辆省份简称字符个数
#define MAX_VL_NUM              5    //最大虚拟线圈个数
#define MAX_DRIVECHAN_NUM       16   //最大车道数
#define MAX_COIL_NUM            3    //最大线圈个数
#define MAX_SIGNALLIGHT_NUM     6   //最大信号灯个数
#define LEN_16                    16
#define LEN_32                    32
#define LEN_64                    64
#define LEN_31                    31 
#define	MAX_LINKAGE_CHAN_NUM      16  //报警联动的通道的最大数量
#define MAX_CABINET_COUNT       8    //最大支持机柜数量
#define MAX_ID_LEN              48
#define MAX_PARKNO_LEN          16
#define MAX_ALARMREASON_LEN     32
#define MAX_UPGRADE_INFO_LEN    48 //获取升级文件匹配信息(模糊升级)
#define MAX_CUSTOMDIR_LEN       32 //自定义目录长度
#define MAX_LED_INFO_LEN        512//LED内容长度
#define MAX_VOICE_INFO_LEN      128//语音播报内容长度
#define MAX_LITLE_INFO_LEN      64 //纸票标题内容长度
#define MAX_CUSTOM_INFO_LEN     64 //纸票自定义信息内容长度
#define MAX_PHONE_NUM_LEN       16 //联系电话内容长度
#define MAX_APP_SERIALNUM_LEN   32 //应用序列号长度

#define AUDIOTALKTYPE_G722       0
#define AUDIOTALKTYPE_G711_MU    1
#define AUDIOTALKTYPE_G711_A     2
#define AUDIOTALKTYPE_MP2L2      5
#define AUDIOTALKTYPE_G726         6
#define AUDIOTALKTYPE_AAC         7
#define AUDIOTALKTYPE_PCM         8
#define AUDIOTALKTYPE_G722C       9
#define AUDIOTALKTYPE_MP3         15

//packet type
#define FILE_HEAD            0 //file head
#define VIDEO_I_FRAME        1 //video I frame
#define VIDEO_B_FRAME        2 //video B frame
#define VIDEO_P_FRAME        3 //video P frame
#define AUDIO_PACKET        10 //audio packet
#define PRIVT_PACKET        11 //private packet
//E frame
#define HIK_H264_E_FRAME    (1 << 6)   // 以前E帧不用了,深P帧也没用到
#define MAX_TRANSPARENT_CHAN_NUM      4   //每个串口允许建立的最大透明通道数
#define MAX_TRANSPARENT_ACCESS_NUM    4   //每个监听端口允许接入的最大主机数

//ITS
#define MAX_PARKING_STATUS       8    //车位状态 0代表无车，1代表有车，2代表压线(优先级最高), 3特殊车位 
#define MAX_PARKING_NUM             4    //一个通道最大4个车位 (从左到右车位 数组0～3)

#define MAX_ITS_SCENE_NUM        16   //最大场景数量
#define MAX_SCENE_TIMESEG_NUM    16   //最大场景时间段数量
#define MAX_IVMS_IP_CHANNEL      128  //最大IP通道数
#define DEVICE_ID_LEN            48   //设备编号长度
#define MONITORSITE_ID_LEN       48   //显示点编号长度
#define MAX_AUXAREA_NUM          16   //辅助区域最大数目
#define MAX_SLAVE_CHANNEL_NUM    16   //最大从通道数量
#define MAX_DEVDESC_LEN          64   //设备描述信息最大长度
#define ILLEGAL_LEN       32      //违法代码长度
#define MAX_TRUCK_AXLE_NUM      10      //货车轴最大数
#define MAX_CATEGORY_LEN        8       //车牌附加信息最大字符
#define SERIAL_NO_LEN           16      //泊车位编号


#define MAX_SECRETKEY_LEN           512     //最大秘钥长度
#define MAX_INDEX_CODE_LEN          64      //最大序号长度
#define MAX_ILLEGAL_LEN          64     //违法代码最大字符长度
#define CODE_LEN        64  //授权码
#define ALIAS_LEN       32  //别名，只读
#define MAX_SCH_TASKS_NUM        10

#define MAX_SERVERID_LEN            64 //最大服务器ID的长度
#define MAX_SERVERDOMAIN_LEN        128 //服务器域名最大长度
#define MAX_AUTHENTICATEID_LEN      64 //认证ID最大长度
#define MAX_AUTHENTICATEPASSWD_LEN  32 //认证密码最大长度
#define MAX_SERVERNAME_LEN          64 //最大服务器用户名 
#define MAX_COMPRESSIONID_LEN       64 //编码ID的最大长度
#define MAX_SIPSERVER_ADDRESS_LEN   128 //SIP服务器地址支持域名和IP地址
//压线报警
#define MAX_PlATE_NO_LEN            32   //车牌号码最大长度 2013-09-27
#define UPNP_PORT_NUM                12      //upnp端口映射端口数目

#define MAX_PEOPLE_DETECTION_NUM    8  //最大人员检测区域数

#define MAX_NOTICE_NUMBER_LEN       32   //公告编号最大长度
#define MAX_NOTICE_THEME_LEN        64   //公告主题最大长度
#define MAX_NOTICE_DETAIL_LEN       1024 //公告详情最大长度
#define MAX_NOTICE_PIC_NUM          6    //公告信息最大图片数量
#define MAX_DEV_NUMBER_LEN          32   //设备编号最大长度
#define LOCK_NAME_LEN                   32  //锁名称


#define HOLIDAY_GROUP_NAME_LEN          32  //假日组名称长度
#define MAX_HOLIDAY_PLAN_NUM            16  //假日组最大假日计划数
#define TEMPLATE_NAME_LEN               32  //计划模板名称长度
#define MAX_HOLIDAY_GROUP_NUM           16   //计划模板最大假日组数
#define DOOR_NAME_LEN                   32  //门名称
#define STRESS_PASSWORD_LEN             8   //胁迫密码长度
#define SUPER_PASSWORD_LEN              8   //胁迫密码长度
#define GROUP_NAME_LEN                  32  //群组名称长度
#define GROUP_COMBINATION_NUM           8   //群组组合数
#define MULTI_CARD_GROUP_NUM            4   //单门最大多重卡组数
#define ACS_CARD_NO_LEN                 32  //门禁卡号长度
#define NET_SDK_EMPLOYEE_NO_LEN         32  //工号长度
#define NET_SDK_UUID_LEN                36  //UUID长度
#define NET_SDK_EHOME_KEY_LEN           32  //EHome Key长度
#define CARD_PASSWORD_LEN               8   //卡密码长度
#define MAX_DOOR_NUM                    32  //最大门数
#define MAX_CARD_RIGHT_PLAN_NUM         4   //卡权限最大计划个数
#define MAX_GROUP_NUM_128               128 //最大群组数
#define MAX_CARD_READER_NUM             64  //最大读卡器数
#define MAX_SNEAK_PATH_NODE             8   //最大后续读卡器数
#define MAX_MULTI_DOOR_INTERLOCK_GROUP  8   //最大多门互锁组数
#define MAX_INTER_LOCK_DOOR_NUM         8   //一个多门互锁组中最大互锁门数
#define MAX_CASE_SENSOR_NUM             8   //最大case sensor触发器数
#define MAX_DOOR_NUM_256                256 //最大门数
#define MAX_READER_ROUTE_NUM            16  //最大刷卡循序路径 
#define MAX_FINGER_PRINT_NUM            10  //最大指纹个数
#define MAX_CARD_READER_NUM_512            512 //最大读卡器数
#define NET_SDK_MULTI_CARD_GROUP_NUM_20     20   //单门最大多重卡组数

#define ERROR_MSG_LEN      32 //下发错误信息
#define MAX_DOOR_CODE_LEN               8 //房间代码长度
#define MAX_LOCK_CODE_LEN               8 //锁代码长度
#define PER_RING_PORT_NUM                2   //每个环的端口数
#define SENSORNAME_LEN                  32  //传感器名称长度
#define MAX_SENSORDESCR_LEN             64  //传感器描述长度
#define MAX_DNS_SERVER_NUM              2 //最大DNS个数
#define SENSORUNIT_LEN                  32 //最大单位长度

#define WEP_KEY_MAX_SIZE                32 //最大WEP加密密钥长度
#define WEP_KEY_MAX_NUM                 4  //最大WEP加密密钥个数
#define WPA_KEY_MAX_SIZE                64 //最大WPA共享密钥长度

#define MAX_SINGLE_FTPPICNAME_LEN       20 //最大单个FTP通道名称
#define MAX_CAMNAME_LEN                 32 //最大通道名称
#define MAX_FTPNAME_NUM                 12 //TFP名称数


#define MAX_IDCODE_LEN      128 //  识别码最大长度
#define MAX_VERSIIN_LEN     64  //版本最大长度
#define MAX_IDCODE_NUM      32  // 识别码个数
#define SDK_LEN_2048        2048
#define SDK_MAX_IP_LEN 48

#define RECT_POINT_NUM                    4    //矩形角数

#define MAX_PUBLIC_KEY_LEN 512 // 最大公钥长度
#define CHIP_SERIALNO_LEN 32 //加密芯片序列号长度
#define ENCRYPT_DEV_ID_LEN        20  //设备ID长度

//MCU相关的
#define MAX_SEARCH_ID_LEN               36  //搜索标识符最大长度
#define TERMINAL_NAME_LEN               64  //终端名称长度
#define MAX_URL_LEN                     512 //URL长度
#define REGISTER_NAME_LEN               64 //终端注册GK名称最大长度

//光纤
#define MAX_PORT_NUM            64  //最大端口数
#define MAX_SINGLE_CARD_PORT_NO 4   //光纤收发器单卡最大端口数
#define MAX_FUNC_CARD_NUM       32  //光纤收发器最大功能卡数
#define MAX_FC_CARD_NUM         33  //光纤收发器最大卡数
#define MAX_REMARKS_LEN         128 //注释最大长度
#define MAX_OUTPUT_PORT_NUM                32    //单路输出包含的最大输出端口数
#define MAX_SINGLE_PORT_RECVCARD_NUM    64    //单个端口连接的最大接收卡数
#define MAX_GAMMA_X_VALUE                256    //GAMMA表X轴取值个数
#define NET_DEV_NAME_LEN        64  //设备名称长度
#define NET_DEV_TYPE_NAME_LEN  64  //设备类型名称长度
#define ABNORMAL_INFO_NUM               4        //异常时间段个数

#define PLAYLIST_NAME_LEN                64            //播放表名称长度 
#define PLAYLIST_ITEM_NUM                64            //播放项数目  

//后端相关
#define NET_SDK_MAX_LOGIN_PASSWORD_LEN           128 //用户登录密码最大长度
#define NET_SDK_MAX_ANSWER_LEN                   256 //安全问题答案最大长度
#define NET_SDK_MAX_QUESTION_LIST_LEN            32//安全问题列表最大长度

#define  MAX_SCREEN_AREA_NUM  128  //屏幕区域最大数量
#define NET_SDK_MAX_THERMOMETRYALGNAME           128//测温算法库版本最大长度
#define NET_SDK_MAX_SHIPSALGNAME                 128//船只算法库版本最大长度
#define NET_SDK_MAX_FIRESALGNAME                 128//火点算法库版本最大长度

#define MAX_PASSPORT_NUM_LEN          16     //最大护照证件号长度
#define MAX_PASSPORT_INFO_LEN         128    //最大护照通用信息长度
#define MAX_PASSPORT_NAME_LEN         64     //最大护照姓名长度
#define MAX_PASSPORT_MONITOR_LEN      1024   //最大护照监护信息长度
#define MAX_NATIONALITY_LEN           16     //最大护照国籍长度
#define MAX_PASSPORT_TYPE_LEN         4      //最大护照证件类型长度
//修正后结束
// struct
//报警和异常处理结构(子结构)(多处使用)
typedef struct
{
    DWORD    dwHandleType;            /*处理方式,处理方式的"或"结果*/
    /*0x00: 无响应*/
    /*0x01: 显示器上警告*/
    /*0x02: 声音警告*/
    /*0x04: 上传中心*/
    /*0x08: 触发报警输出*/
    /*0x10: Jpeg抓图并上传EMail*/
    BYTE byRelAlarmOut[MAX_ALARMOUT];  //报警触发的输出通道,报警触发的输出,为1表示触发该输出
}NET_DVR_HANDLEEXCEPTION, *LPNET_DVR_HANDLEEXCEPTION;
//时间段(子结构)
typedef struct
{
    //开始时间
    BYTE byStartHour;
    BYTE byStartMin;
    //结束时间
    BYTE byStopHour;
    BYTE byStopMin;
}NET_DVR_SCHEDTIME, *LPNET_DVR_SCHEDTIME;

typedef struct
{
    BYTE byBrightness;      /*亮度,0-255*/
    BYTE byContrast;        /*对比度,0-255*/    
    BYTE bySaturation;      /*饱和度,0-255*/
    BYTE byHue;                /*色调,0-255*/
}NET_DVR_COLOR, *LPNET_DVR_COLOR;

//NET_DVR_Login_V30()参数结构
typedef struct tagNET_DVR_DEVICEINFO_V30
{
    BYTE sSerialNumber[SERIALNO_LEN];  //序列号
    BYTE byAlarmInPortNum;                //报警输入个数
    BYTE byAlarmOutPortNum;                //报警输出个数
    BYTE byDiskNum;                    //硬盘个数
    BYTE byDVRType;                    //设备类型, 1:DVR 2:ATM DVR 3:DVS ......
    BYTE byChanNum;                    //模拟通道个数
    BYTE byStartChan;                    //起始通道号,例如DVS-1,DVR - 1
    BYTE byAudioChanNum;                //语音通道数
    BYTE byIPChanNum;                    //最大数字通道个数，低位
    BYTE byZeroChanNum;            //零通道编码个数 //2010-01-16
    BYTE byMainProto;            //主码流传输协议类型 0-private, 1-rtsp,2-同时支持private和rtsp
    BYTE bySubProto;                //子码流传输协议类型0-private, 1-rtsp,2-同时支持private和rtsp
    BYTE bySupport;
    BYTE bySupport1;
    BYTE bySupport2;
    WORD wDevType;
    BYTE bySupport3;
    BYTE byMultiStreamProto;//是否支持多码流,按位表示,0-不支持,1-支持,bit1-码流3,bit2-码流4,bit7-主码流，bit-8子码流
    BYTE byStartDChan;        //起始数字通道号,0表示无效
    BYTE byStartDTalkChan;    //起始数字对讲通道号，区别于模拟对讲通道号，0表示无效
    BYTE byHighDChanNum;        //数字通道个数，高位
    BYTE bySupport4;
    BYTE byLanguageType;
    BYTE byVoiceInChanNum;   //音频输入通道数
    BYTE byStartVoiceInChanNo; //音频输入起始通道号 0表示无效
    BYTE  bySupport5;
    BYTE  bySupport6;
    BYTE  byMirrorChanNum;    //镜像通道个数，<录播主机中用于表示导播通道>
    WORD wStartMirrorChanNo;  //起始镜像通道号
    BYTE bySupport7;
    BYTE  byRes2;        //保留
}NET_DVR_DEVICEINFO_V30, *LPNET_DVR_DEVICEINFO_V30;

typedef struct tagNET_DVR_DEVICEINFO_V40
{
    NET_DVR_DEVICEINFO_V30 struDeviceV30;
    BYTE  bySupportLock;        //设备支持锁定功能，该字段由SDK根据设备返回值来赋值的。bySupportLock为1时，dwSurplusLockTime和byRetryLoginTime有效
    BYTE  byRetryLoginTime;        //剩余可尝试登陆的次数，用户名，密码错误时，此参数有效
    BYTE  byPasswordLevel;      //admin密码安全等级0-无效，1-默认密码，2-有效密码,3-风险较高的密码。当用户的密码为出厂默认密码（12345）或者风险较高的密码时，上层客户端需要提示用户更改密码。
    BYTE  byProxyType;  //代理类型，0-不使用代理, 1-使用socks5代理, 2-使用EHome代理
    DWORD dwSurplusLockTime;    //剩余时间，单位秒，用户锁定时，此参数有效
    BYTE  byCharEncodeType;     //字符编码类型
    BYTE  bySupportDev5;//支持v50版本的设备参数获取，设备名称和设备类型名称长度扩展为64字节
    BYTE  bySupport;  //能力集扩展，位与结果：0- 不支持，1- 支持
    BYTE  byLoginMode; //登录模式 0-Private登录 1-ISAPI登录
    DWORD dwOEMCode;
    int iResidualValidity;   //该用户密码剩余有效天数，单位：天，返回负值，表示密码已经超期使用，例如“-3表示密码已经超期使用3天”
    BYTE  byResidualValidity; // iResidualValidity字段是否有效，0-无效，1-有效
    BYTE  byRes2[243];
}NET_DVR_DEVICEINFO_V40, *LPNET_DVR_DEVICEINFO_V40;

typedef void (*fLoginResultCallBack) (LONG lUserID, DWORD dwResult, LPNET_DVR_DEVICEINFO_V30 lpDeviceInfo , void* pUser);
typedef void (*REALDATACALLBACK) (LONG lPlayHandle, DWORD dwDataType, BYTE *pBuffer, DWORD dwBufSize, void* pUser);

typedef struct tagNET_DVR_USER_LOGIN_INFO
{
    char sDeviceAddress[NET_DVR_DEV_ADDRESS_MAX_LEN];
    BYTE byUseTransport;    //是否启用能力集透传，0--不启用透传，默认，1--启用透传
    WORD wPort;
    char sUserName[NET_DVR_LOGIN_USERNAME_MAX_LEN];
    char sPassword[NET_DVR_LOGIN_PASSWD_MAX_LEN];
    fLoginResultCallBack cbLoginResult;
    void *pUser;
    BOOL bUseAsynLogin;
    BYTE byProxyType; //0:不使用代理，1：使用标准代理，2：使用EHome代理
    BYTE byUseUTCTime;    //0-不进行转换，默认,1-接口上输入输出全部使用UTC时间,SDK完成UTC时间与设备时区的转换,2-接口上输入输出全部使用平台本地时间，SDK完成平台本地时间与设备时区的转换
    BYTE byLoginMode; //0-Private 1-ISAPI 2-自适应
    BYTE byHttps;    //0-不适用tls，1-使用tls 2-自适应
    LONG iProxyID;    //代理服务器序号，添加代理服务器信息时，相对应的服务器数组下表值
    BYTE byVerifyMode;  //认证方式，0-不认证，1-双向认证，2-单向认证；认证仅在使用TLS的时候生效;
    BYTE byRes3[119];
}NET_DVR_USER_LOGIN_INFO,*LPNET_DVR_USER_LOGIN_INFO;

//图片质量
typedef struct tagNET_DVR_JPEGPARA
{
    WORD    wPicSize;
    WORD    wPicQuality;            /* 图片质量系数 0-最好 1-较好 2-一般 */
}NET_DVR_JPEGPARA, *LPNET_DVR_JPEGPARA;

//软解码预览参数
typedef struct tagNET_DVR_CLIENTINFO
{
    LONG lChannel;
    LONG lLinkMode;
    HWND hPlayWnd;
    char* sMultiCastIP;
    BYTE byProtoType;
    BYTE byRes[3];
}NET_DVR_CLIENTINFO, *LPNET_DVR_CLIENTINFO;

#define STREAM_ID_LEN   32

//预览V40接口
typedef struct tagNET_DVR_PREVIEWINFO
{
    LONG lChannel;
    DWORD dwStreamType;
    DWORD dwLinkMode;
    HWND hPlayWnd;
    DWORD bBlocked;
    DWORD bPassbackRecord;
    BYTE byPreviewMode;
    BYTE byStreamID[STREAM_ID_LEN];
    BYTE byProtoType;
    BYTE byRes1;
    BYTE byVideoCodingType;
    DWORD dwDisplayBufNum;
    BYTE byNPQMode;
    BYTE byRes[215];
}NET_DVR_PREVIEWINFO, *LPNET_DVR_PREVIEWINFO;

typedef struct tagNET_DVR_BUF_INFO
{
    void*   pBuf;    //缓冲区指针
    DWORD   nLen;    //缓冲区长度
}NET_DVR_BUF_INFO, *LPNET_DVR_BUF_INFO;

typedef struct tagNET_DVR_IN_PARAM
{
    NET_DVR_BUF_INFO struCondBuf;
    NET_DVR_BUF_INFO struInParamBuf;
    DWORD  dwRecvTimeout;
    BYTE   byRes[32];
}NET_DVR_IN_PARAM,LPNET_DVR_IN_PARAM;

typedef struct tagNET_DVR_OUT_PARAM
{
    NET_DVR_BUF_INFO struOutBuf;
    void*  lpStatusList;
    BYTE   byRes[32];
}NET_DVR_OUT_PARAM,LPNET_DVR_OUT_PARAM;


typedef struct tagNET_DVR_SETUPALARM_PARAM
{
    DWORD dwSize;
    BYTE  byLevel; //布防优先级，0-一等级（高），1-二等级（中），2-三等级（低）
    BYTE  byAlarmInfoType; //上传报警信息类型（抓拍机支持），0-老报警信息（NET_DVR_PLATE_RESULT），1-新报警信息(NET_ITS_PLATE_RESULT)2012-9-28
    BYTE  byRetAlarmTypeV40; //0--返回NET_DVR_ALARMINFO_V30或NET_DVR_ALARMINFO, 1--设备支持NET_DVR_ALARMINFO_V40则返回NET_DVR_ALARMINFO_V40，不支持则返回NET_DVR_ALARMINFO_V30或NET_DVR_ALARMINFO
    BYTE  byRetDevInfoVersion; //CVR上传报警信息回调结构体版本号 0-COMM_ALARM_DEVICE， 1-COMM_ALARM_DEVICE_V40
    BYTE  byRetVQDAlarmType; //VQD报警上传类型，0-上传报报警NET_DVR_VQD_DIAGNOSE_INFO，1-上传报警NET_DVR_VQD_ALARM
    //1-表示人脸侦测报警扩展(INTER_FACE_DETECTION),0-表示原先支持结构(INTER_FACESNAP_RESULT)
    BYTE  byFaceAlarmDetection;
    //Bit0- 表示二级布防是否上传图片: 0-上传，1-不上传
    //Bit1- 表示开启数据上传确认机制；0-不开启，1-开启
    //Bit6- 表示雷达检测报警(eventType:radarDetection)是否开启实时上传；0-不开启，1-开启（用于web插件实时显示雷达目标轨迹）
    BYTE  bySupport;
    //断网续传类型
    //bit0-车牌检测（IPC） （0-不续传，1-续传）
    //bit1-客流统计（IPC）  （0-不续传，1-续传）
    //bit2-热度图统计（IPC） （0-不续传，1-续传）
    //bit3-人脸抓拍（IPC） （0-不续传，1-续传）
    //bit4-人脸对比（IPC） （0-不续传，1-续传）
    BYTE  byBrokenNetHttp;
    WORD  wTaskNo;    //任务处理号 和 (上传数据NET_DVR_VEHICLE_RECOG_RESULT中的字段dwTaskNo对应 同时 下发任务结构 NET_DVR_VEHICLE_RECOG_COND中的字段dwTaskNo对应)
    BYTE  byDeployType;    //布防类型：0-客户端布防，1-实时布防
    BYTE  bySubScription;	//订阅，按位表示，未开启订阅不上报  //占位
    //Bit7-移动侦测人车分类是否传图；0-不传图(V30上报)，1-传图(V40上报)
    BYTE  byRes1[2];
    BYTE  byAlarmTypeURL;//bit0-表示人脸抓拍报警上传（INTER_FACESNAP_RESULT）；0-表示二进制传输，1-表示URL传输（设备支持的情况下，设备支持能力根据具体报警能力集判断,同时设备需要支持URL的相关服务，当前是”云存储“）
    //bit1-表示EVENT_JSON中图片数据长传类型；0-表示二进制传输，1-表示URL传输（设备支持的情况下，设备支持能力根据具体报警能力集判断）
    //bit2 - 人脸比对(报警类型为COMM_SNAP_MATCH_ALARM)中图片数据上传类型：0 - 二进制传输，1 - URL传输
    //bit3 - 行为分析(报警类型为COMM_ALARM_RULE)中图片数据上传类型：0 - 二进制传输，1 - URL传输，本字段设备是否支持，对应软硬件能力集中<isSupportBehaviorUploadByCloudStorageURL>节点是否返回且为true
    BYTE  byCustomCtrl;//Bit0- 表示支持副驾驶人脸子图上传: 0-不上传,1-上传
}NET_DVR_SETUPALARM_PARAM, *LPNET_DVR_SETUPALARM_PARAM;


//单IO触发抓拍功能配置
typedef struct tagNET_DVR_SNAPCFG
{
    DWORD   dwSize;
    BYTE    byRelatedDriveWay;//触发IO关联的车道号
    BYTE     bySnapTimes; //线圈抓拍次数，0-不抓拍，非0-连拍次数，目前最大5次
    WORD    wSnapWaitTime;  //抓拍等待时间，单位ms，取值范围[0,60000]
    WORD    wIntervalTime[MAX_INTERVAL_NUM];//连拍间隔时间，ms
    DWORD   dwSnapVehicleNum; //抓拍车辆序号。
    NET_DVR_JPEGPARA  struJpegPara;//抓拍图片参数
    BYTE    byRes2[16];//保留字节
}NET_DVR_SNAPCFG, *LPNET_DVR_SNAPCFG;


//报警设备信息
typedef struct
{
    BYTE byUserIDValid;                 /* userid是否有效 0-无效，1-有效 */
    BYTE bySerialValid;                 /* 序列号是否有效 0-无效，1-有效 */
    BYTE byVersionValid;                /* 版本号是否有效 0-无效，1-有效 */
    BYTE byDeviceNameValid;             /* 设备名字是否有效 0-无效，1-有效 */
    BYTE byMacAddrValid;                /* MAC地址是否有效 0-无效，1-有效 */
    BYTE byLinkPortValid;               /* login端口是否有效 0-无效，1-有效 */
    BYTE byDeviceIPValid;               /* 设备IP是否有效 0-无效，1-有效 */
    BYTE bySocketIPValid;               /* socket ip是否有效 0-无效，1-有效 */
    LONG lUserID;                       /* NET_DVR_Login()返回值, 布防时有效 */
    BYTE sSerialNumber[SERIALNO_LEN];    /* 序列号 */
    DWORD dwDeviceVersion;                /* 版本信息 高16位表示主版本，低16位表示次版本*/
    char sDeviceName[NAME_LEN];            /* 设备名字 */
    BYTE byMacAddr[MACADDR_LEN];        /* MAC地址 */
    WORD wLinkPort;                     /* link port */
    char sDeviceIP[128];                /* IP地址 */
    char sSocketIP[128];                /* 报警主动上传时的socket IP地址 */
    BYTE byIpProtocol;                  /* Ip协议 0-IPV4, 1-IPV6 */
    BYTE byRes1[2];
    BYTE bJSONBroken;                   //JSON断网续传标志。0：不续传；1：续传
    WORD wSocketPort;
    BYTE byRes2[6];
}NET_DVR_ALARMER, *LPNET_DVR_ALARMER;

//信号丢失报警(子结构)
typedef struct 
{
    BYTE byEnableHandleVILost;    /* 是否处理信号丢失报警 */
    NET_DVR_HANDLEEXCEPTION strVILostHandleType;    /* 处理方式 */
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT];//布防时间
}NET_DVR_VILOST, *LPNET_DVR_VILOST;

//移动侦测(子结构)
typedef struct 
{
    BYTE byMotionScope[18][22];    /*侦测区域,共有22*18个小宏块,为1表示改宏块是移动侦测区域,0-表示不是*/
    BYTE byMotionSensitive;        /*移动侦测灵敏度, 0 - 5,越高越灵敏,0xff关闭*/
    BYTE byEnableHandleMotion;    /* 是否处理移动侦测 */
    BYTE byEnableDisplay;    /*启用移动侦测高亮显示，0-否，1-是*/
    char reservedData;
    NET_DVR_HANDLEEXCEPTION strMotionHandleType;    /* 处理方式 */
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT];//布防时间
    BYTE byRelRecordChan[MAX_CHANNUM]; //报警触发的录象通道,为1表示触发该通道
}NET_DVR_MOTION, *LPNET_DVR_MOTION;
//遮挡报警(子结构)  区域大小704*576
typedef struct 
{
    DWORD dwEnableHideAlarm;                /* 是否启动遮挡报警 ,0-否,1-低灵敏度 2-中灵敏度 3-高灵敏度*/
    WORD wHideAlarmAreaTopLeftX;            /* 遮挡区域的x坐标 */
    WORD wHideAlarmAreaTopLeftY;            /* 遮挡区域的y坐标 */
    WORD wHideAlarmAreaWidth;                /* 遮挡区域的宽 */
    WORD wHideAlarmAreaHeight;                /*遮挡区域的高*/
    NET_DVR_HANDLEEXCEPTION strHideAlarmHandleType;    /* 处理方式 */
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT];//布防时间
}NET_DVR_HIDEALARM, *LPNET_DVR_HIDEALARM;

typedef struct
{
    NET_DVR_COLOR      struColor[MAX_TIMESEGMENT_V30];/*图象参数(第一个有效，其他三个保留)*/
    NET_DVR_SCHEDTIME  struHandleTime[MAX_TIMESEGMENT_V30];/*处理时间段(保留)*/
}NET_DVR_VICOLOR, *LPNET_DVR_VICOLOR;

//遮挡区域(子结构)
typedef struct 
{
    WORD wHideAreaTopLeftX;                /* 遮挡区域的x坐标 */
    WORD wHideAreaTopLeftY;                /* 遮挡区域的y坐标 */
    WORD wHideAreaWidth;                /* 遮挡区域的宽 */
    WORD wHideAreaHeight;                /*遮挡区域的高*/
}NET_DVR_SHELTER, *LPNET_DVR_SHELTER;

typedef struct
{
    BYTE byRed;        //RGB颜色三分量中的红色
    BYTE byGreen;    //RGB颜色三分量中的绿色
    BYTE byBlue;    //RGB颜色三分量中的蓝色
    BYTE byRes;        //保留
}NET_DVR_RGB_COLOR, *LPNET_DVR_RGB_COLOR;

typedef struct
{
    BYTE byObjectSize;//占比参数(0~100)
    BYTE byMotionSensitive; /*移动侦测灵敏度, 0 - 5,越高越灵敏,0xff关闭*/
    BYTE byRes[6];
}NET_DVR_DNMODE, *LPNET_DVR_DNMODE;

//区域框结构
typedef struct tagNET_VCA_RECT
{
    float fX;               //边界框左上角点的X轴坐标, 0.000~1
    float fY;               //边界框左上角点的Y轴坐标, 0.000~1
    float fWidth;           //边界框的宽度, 0.000~1
    float fHeight;          //边界框的高度, 0.000~1
}NET_VCA_RECT, *LPNET_VCA_RECT;

//球机位置信息
typedef struct
{
    WORD wAction;//获取时该字段无效
    WORD wPanPos;//水平参数
    WORD wTiltPos;//垂直参数
    WORD wZoomPos;//变倍参数
}NET_DVR_PTZPOS, *LPNET_DVR_PTZPOS;
//球机范围信息
typedef struct
{
    WORD wPanPosMin;//水平参数min
    WORD wPanPosMax;//水平参数max
    WORD wTiltPosMin;//垂直参数min
    WORD wTiltPosMax;//垂直参数max
    WORD wZoomPosMin;//变倍参数min
    WORD wZoomPosMax;//变倍参数max
}NET_DVR_PTZSCOPE, *LPNET_DVR_PTZSCOPE;

typedef struct 
{
    BYTE byAreaNo;//区域编号(IPC- 1~8)
    BYTE byRes[3];
    NET_VCA_RECT struRect;//单个区域的坐标信息(矩形) size = 16;
    NET_DVR_DNMODE  struDayNightDisable;//关闭模式
    NET_DVR_DNMODE  struDayModeParam;//白天模式
    NET_DVR_DNMODE  struNightModeParam;//夜晚模式
    BYTE byRes1[8];
}NET_DVR_MOTION_MULTI_AREAPARAM, *LPNET_DVR_MOTION_MULTI_AREAPARAM;

typedef struct  
{
    BYTE byHour;//0~24
    BYTE byMinute;//0~60
    BYTE bySecond;//0~60
    BYTE byRes;
    WORD wMilliSecond; //0~1000
    BYTE byRes1[2];
}NET_DVR_DAYTIME,*LPNET_DVR_DAYTIME;

typedef struct
{
    NET_DVR_DAYTIME  struStartTime; //开始时间
    NET_DVR_DAYTIME  struStopTime; //结束时间
}NET_DVR_SCHEDULE_DAYTIME, *LPNET_DVR_SCHEDULE_DAYTIME;

typedef struct
{
    BYTE byDayNightCtrl;//日夜控制 0~关闭,1~自动切换,2~定时切换(默认关闭)
    BYTE byAllMotionSensitive; /*移动侦测灵敏度, 0 - 5,越高越灵敏,0xff关闭，全部区域的灵敏度范围*/ 
    BYTE byRes[2];//
    NET_DVR_SCHEDULE_DAYTIME struScheduleTime;//切换时间  16
    NET_DVR_MOTION_MULTI_AREAPARAM struMotionMultiAreaParam[MAX_MULTI_AREA_NUM];//最大支持24个区域
    BYTE byRes1[60];
}NET_DVR_MOTION_MULTI_AREA,*LPNET_DVR_MOTION_MULTI_AREA; //1328

typedef struct
{
    BYTE byMotionScope[64][96];        /*侦测区域,0-96位,表示64行,共有96*64个小宏块,目前有效的是22*18,为1表示是移动侦测区域,0-表示不是*/
    BYTE byMotionSensitive;            /*移动侦测灵敏度, 0 - 5,越高越灵敏,0xff关闭*/
    BYTE byRes[3];
}NET_DVR_MOTION_SINGLE_AREA, *LPNET_DVR_MOTION_SINGLE_AREA;

typedef struct 
{
    NET_DVR_MOTION_SINGLE_AREA  struMotionSingleArea; //普通模式下的单区域设
    NET_DVR_MOTION_MULTI_AREA struMotionMultiArea; //专家模式下的多区域设置    
}NET_DVR_MOTION_MODE_PARAM, *LPNET_DVR_MOTION_MODE_PARAM;

typedef struct
{
    DWORD dwEnableHideAlarm;                /* 是否启动遮挡报警，0-否，1-低灵敏度，2-中灵敏度，3-高灵敏度*/
    WORD wHideAlarmAreaTopLeftX;            /* 遮挡区域的x坐标 */
    WORD wHideAlarmAreaTopLeftY;            /* 遮挡区域的y坐标 */
    WORD wHideAlarmAreaWidth;                /* 遮挡区域的宽 */
    WORD wHideAlarmAreaHeight;                /*遮挡区域的高*/ 
    /* 信号丢失触发报警输出 */    
    DWORD   dwHandleType;        //异常处理,异常处理方式的"或"结果   
    /*0x00: 无响应*/
    /*0x01: 显示器上警告*/
    /*0x02: 声音警告*/
    /*0x04: 上传中心*/
    /*0x08: 触发报警输出*/
    /*0x10: 触发JPRG抓图并上传Email*/
    /*0x20: 无线声光报警器联动*/
    /*0x40: 联动电子地图(目前只有PCNVR支持)*/
    /*0x200: 抓图并上传FTP*/ 
    /*0x1000:抓图上传到云*/
    DWORD   dwMaxRelAlarmOutChanNum ; //触发的报警输出通道数（只读）最大支持数量
    DWORD   dwRelAlarmOut[MAX_ALARMOUT_V40]; /*触发报警输出号，按值表示,采用紧凑型排列，从下标0 - dwRelAlarmOut -1有效，如果中间遇到0xffffffff,则后续无效*/  
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT_V30]; /*布防时间*/
    BYTE  byRes[64]; //保留
}NET_DVR_HIDEALARM_V40,*LPNET_DVR_HIDEALARM_V40; //遮挡报警

typedef struct 
{    
    NET_DVR_MOTION_MODE_PARAM  struMotionMode; //(5.1.0新增)
    BYTE byEnableHandleMotion;        /* 是否处理移动侦测 0－否 1－是*/ 
    BYTE byEnableDisplay;    /*启用移动侦测高亮显示，0-否，1-是*/
    BYTE byConfigurationMode; //0~普通,1~专家(5.1.0新增)
    BYTE byKeyingEnable; //启用键控移动侦测 0-不启用，1-启用
    /* 异常处理方式 */
    DWORD   dwHandleType;        //异常处理,异常处理方式的"或"结果   
    /*0x00: 无响应*/
    /*0x01: 显示器上警告*/
    /*0x02: 声音警告*/
    /*0x04: 上传中心*/
    /*0x08: 触发报警输出*/
    /*0x10: 触发JPRG抓图并上传Email*/
    /*0x20: 无线声光报警器联动*/
    /*0x40: 联动电子地图(目前只有PCNVR支持)*/
    /*0x200: 抓图并上传FTP*/ 
    /*0x1000: 抓图上传到云*/
    DWORD   dwMaxRelAlarmOutChanNum ; //触发的报警输出通道数（只读）最大支持数量
    DWORD   dwRelAlarmOut[MAX_ALARMOUT_V40]; //实际触发的报警输出号，按值表示,采用紧凑型排列，从下标0 - dwRelAlarmOut -1有效，如果中间遇到0xffffffff,则后续无效
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT_V30]; /*布防时间*/
    /*触发的录像通道*/
    DWORD     dwMaxRecordChanNum;   //设备支持的最大关联录像通道数-只读
    DWORD     dwRelRecordChan[MAX_CHANNUM_V40];     /* 实际触发录像通道，按值表示,采用紧凑型排列，从下标0 - dwRelRecordChan -1有效，如果中间遇到0xffffffff,则后续无效*/  
    BYTE  byDiscardFalseAlarm; //启用去误报 0-无效，1-不启用，2-启用
    BYTE  byRes[127]; //保留字节
}NET_DVR_MOTION_V40,*LPNET_DVR_MOTION_V40;

typedef struct
{
    DWORD dwEnableVILostAlarm;                /* 是否启动信号丢失报警 ,0-否,1-是*/
    /* 信号丢失触发报警输出 */    
    DWORD   dwHandleType;        //异常处理,异常处理方式的"或"结果   
    /*0x00: 无响应*/
    /*0x01: 显示器上警告*/
    /*0x02: 声音警告*/
    /*0x04: 上传中心*/
    /*0x08: 触发报警输出*/
    /*0x10: 触发JPRG抓图并上传Email*/
    /*0x20: 无线声光报警器联动*/
    /*0x40: 联动电子地图(目前只有PCNVR支持)*/
    /*0x200: 抓图并上传FTP*/ 
    /*0x1000:抓图上传到云*/
    DWORD   dwMaxRelAlarmOutChanNum ; //触发的报警输出通道数（只读）最大支持数量
    DWORD   dwRelAlarmOut[MAX_ALARMOUT_V40]; /*触发报警输出号，按值表示,采用紧凑型排列，从下标0 - dwRelAlarmOut -1有效，如果中间遇到0xffffffff,则后续无效*/
    NET_DVR_SCHEDTIME struAlarmTime[MAX_DAYS][MAX_TIMESEGMENT_V30]; /*布防时间*/
    BYTE    byVILostAlarmThreshold;    /*信号丢失报警阈值，当值低于阈值，认为信号丢失，取值0-99*/
    BYTE    byRes[63]; //保留
}NET_DVR_VILOST_V40,*LPNET_DVR_VILOST_V40;    //信号丢失报警

//DVR设备参数
typedef struct
{
    DWORD dwSize;
    BYTE sDVRName[NAME_LEN];     //DVR名称
    DWORD dwDVRID;                //DVR ID,用于遥控器 //V1.4(0-99), V1.5(0-255)
    DWORD dwRecycleRecord;        //是否循环录像,0:不是; 1:是
    //以下不可更改
    BYTE sSerialNumber[SERIALNO_LEN];  //序列号
    DWORD dwSoftwareVersion;            //软件版本号,高16位是主版本,低16位是次版本
    DWORD dwSoftwareBuildDate;            //软件生成日期,0xYYYYMMDD
    DWORD dwDSPSoftwareVersion;            //DSP软件版本,高16位是主版本,低16位是次版本
    DWORD dwDSPSoftwareBuildDate;        // DSP软件生成日期,0xYYYYMMDD
    DWORD dwPanelVersion;                // 前面板版本,高16位是主版本,低16位是次版本
    DWORD dwHardwareVersion;    // 硬件版本,高16位是主版本,低16位是次版本
    BYTE byAlarmInPortNum;        //DVR报警输入个数
    BYTE byAlarmOutPortNum;        //DVR报警输出个数
    BYTE byRS232Num;            //DVR 232串口个数
    BYTE byRS485Num;            //DVR 485串口个数
    BYTE byNetworkPortNum;        //网络口个数
    BYTE byDiskCtrlNum;            //DVR 硬盘控制器个数
    BYTE byDiskNum;                //DVR 硬盘个数
    BYTE byDVRType;                //DVR类型, 1:DVR 2:ATM DVR 3:DVS ......
    BYTE byChanNum;                //DVR 通道个数
    BYTE byStartChan;            //起始通道号,例如DVS-1,DVR - 1
    BYTE byDecordChans;            //DVR 解码路数
    BYTE byVGANum;                //VGA口的个数
    BYTE byUSBNum;                //USB口的个数
    BYTE byAuxoutNum;            //辅口的个数
    BYTE byAudioNum;            //语音口的个数
    BYTE byIPChanNum;            //最大数字通道数
}NET_DVR_DEVICECFG, *LPNET_DVR_DEVICECFG;


//DVR设备参数
typedef struct
{
    DWORD dwSize;
    BYTE sDVRName[NAME_LEN];     //DVR名称
    DWORD dwDVRID;                //DVR ID,用于遥控器 //V1.4(0-99), V1.5(0-255)
    DWORD dwRecycleRecord;        //是否循环录像,0:不是; 1:是
    //以下不可更改
    BYTE sSerialNumber[SERIALNO_LEN];  //序列号
    DWORD dwSoftwareVersion;            //软件版本号,高16位是主版本,低16位是次版本
    DWORD dwSoftwareBuildDate;            //软件生成日期,0xYYYYMMDD
    DWORD dwDSPSoftwareVersion;            //DSP软件版本,高16位是主版本,低16位是次版本
    DWORD dwDSPSoftwareBuildDate;        // DSP软件生成日期,0xYYYYMMDD
    DWORD dwPanelVersion;                // 前面板版本,高16位是主版本,低16位是次版本
    DWORD dwHardwareVersion;    // 硬件版本,高16位是主版本,低16位是次版本
    BYTE byAlarmInPortNum;        //DVR报警输入个数
    BYTE byAlarmOutPortNum;        //DVR报警输出个数
    BYTE byRS232Num;            //DVR 232串口个数
    BYTE byRS485Num;            //DVR 485串口个数 
    BYTE byNetworkPortNum;        //网络口个数
    BYTE byDiskCtrlNum;            //DVR 硬盘控制器个数
    BYTE byDiskNum;                //DVR 硬盘个数
    BYTE byDVRType;                //DVR类型, 1:DVR 2:ATM DVR 3:DVS ......
    BYTE byChanNum;                //DVR 通道个数
    BYTE byStartChan;            //起始通道号,例如DVS-1,DVR - 1
    BYTE byDecordChans;            //DVR 解码路数
    BYTE byVGANum;                //VGA口的个数 
    BYTE byUSBNum;                //USB口的个数
    BYTE byAuxoutNum;            //辅口的个数
    BYTE byAudioNum;            //语音口的个数
    BYTE byIPChanNum;            //最大数字通道数 低8位，高8位见byHighIPChanNum 
    BYTE byZeroChanNum;            //零通道编码个数
    BYTE bySupport;        //能力，位与结果为0表示不支持，1表示支持，
    //bySupport & 0x1, 表示是否支持智能搜索
    //bySupport & 0x2, 表示是否支持备份
    //bySupport & 0x4, 表示是否支持压缩参数能力获取
    //bySupport & 0x8, 表示是否支持多网卡
    //bySupport & 0x10, 表示支持远程SADP
    //bySupport & 0x20, 表示支持Raid卡功能
    //bySupport & 0x40, 表示支持IPSAN搜索
    //bySupport & 0x80, 表示支持rtp over rtsp
    BYTE byEsataUseage;        //Esata的默认用途，0-默认备份，1-默认录像
    BYTE byIPCPlug;            //0-关闭即插即用，1-打开即插即用
    BYTE byStorageMode;        //0-盘组模式,1-磁盘配额, 2抽帧模式, 3-自动
    BYTE bySupport1;        //能力，位与结果为0表示不支持，1表示支持
    //bySupport1 & 0x1, 表示是否支持snmp v30
    //bySupport1 & 0x2, 支持区分回放和下载
    //bySupport1 & 0x4, 是否支持布防优先级    
    //bySupport1 & 0x8, 智能设备是否支持布防时间段扩展
    //bySupport1 & 0x10, 表示是否支持多磁盘数（超过33个）
    //bySupport1 & 0x20, 表示是否支持rtsp over http    
    WORD wDevType;//设备型号
    BYTE  byDevTypeName[DEV_TYPE_NAME_LEN];//设备型号名称 
    BYTE bySupport2; //能力集扩展，位与结果为0表示不支持，1表示支持
    //bySupport2 & 0x1, 表示是否支持扩展的OSD字符叠加(终端和抓拍机扩展区分)
    BYTE byAnalogAlarmInPortNum; //模拟报警输入个数
    BYTE byStartAlarmInNo;    //模拟报警输入起始号
    BYTE byStartAlarmOutNo;  //模拟报警输出起始号
    BYTE  byStartIPAlarmInNo;  //IP报警输入起始号
    BYTE  byStartIPAlarmOutNo; //IP报警输出起始号
    BYTE byHighIPChanNum;      //数字通道个数，高8位 
    BYTE byEnableRemotePowerOn;//是否启用在设备休眠的状态下远程开机功能，0-不启用，1-启用
    WORD wDevClass; //设备大类备是属于哪个产品线，0 保留，1-50 DVR，51-100 DVS，101-150 NVR，151-200 IPC，65534 其他，具体分类方法见《设备类型对应序列号和类型值.docx》
    BYTE byRes2[6];    //保留
}NET_DVR_DEVICECFG_V40, *LPNET_DVR_DEVICECFG_V40;

//通道图象结构(SDK_V13及之前版本)
typedef struct 
{
    DWORD dwSize;
    BYTE sChanName[NAME_LEN];
    DWORD dwVideoFormat;    /* 只读 视频制式 1-NTSC 2-PAL*/
    BYTE byBrightness;      /*亮度,0-255*/
    BYTE byContrast;        /*对比度,0-255*/    
    BYTE bySaturation;      /*饱和度,0-255 */
    BYTE byHue;                /*色调,0-255*/
    //显示通道名
    DWORD dwShowChanName; // 预览的图象上是否显示通道名称,0-不显示,1-显示 区域大小704*576
    WORD wShowNameTopLeftX;                /* 通道名称显示位置的x坐标 */
    WORD wShowNameTopLeftY;                /* 通道名称显示位置的y坐标 */
    //信号丢失报警
    NET_DVR_VILOST struVILost;
    //移动侦测
    NET_DVR_MOTION struMotion;
    //遮挡报警
    NET_DVR_HIDEALARM struHideAlarm;
    //遮挡  区域大小704*576
    DWORD dwEnableHide;        /* 是否启动遮挡 ,0-否,1-是*/
    WORD wHideAreaTopLeftX;                /* 遮挡区域的x坐标 */
    WORD wHideAreaTopLeftY;                /* 遮挡区域的y坐标 */
    WORD wHideAreaWidth;                /* 遮挡区域的宽 */
    WORD wHideAreaHeight;                /*遮挡区域的高*/
    //OSD
    DWORD dwShowOsd;// 预览的图象上是否显示OSD,0-不显示,1-显示 区域大小704*576
    WORD wOSDTopLeftX;                /* OSD的x坐标 */
    WORD wOSDTopLeftY;                /* OSD的y坐标 */
    BYTE byOSDType;                    /* OSD类型(主要是年月日格式) */
    /* 0: XXXX-XX-XX 年月日 */
    /* 1: XX-XX-XXXX 月日年 */
    /* 2: XXXX年XX月XX日 */
    /* 3: XX月XX日XXXX年 */
    /* 4: XX-XX-XXXX 日月年*/
    /* 5: XX日XX月XXXX年 */
    /*6: xx/xx/xxxx(月/日/年) */
    /*7: xxxx/xx/xx(年/月/日) */
    /*8: xx/xx/xxxx(日/月/年)*/
    BYTE byDispWeek;                /* 是否显示星期 */
    BYTE byOSDAttrib;                /* OSD属性:透明，闪烁 */
    /* 1: 透明,闪烁 */
    /* 2: 透明,不闪烁 */
    /* 3: 闪烁,不透明 */
    /* 4: 不透明,不闪烁 */
    char reservedData2;
}NET_DVR_PICCFG, *LPNET_DVR_PICCFG;


typedef struct
{
    DWORD  dwSize;
    BYTE  sChanName[NAME_LEN]; 
    DWORD  dwVideoFormat;    /* 只读 视频制式 1-NTSC 2-PAL  */
    NET_DVR_VICOLOR struViColor;//    图像参数按时间段设置
    //显示通道名
    DWORD  dwShowChanName; // 预览的图象上是否显示通道名称,0-不显示,1-显示
    WORD    wShowNameTopLeftX;                /* 通道名称显示位置的x坐标 */
    WORD    wShowNameTopLeftY;                /* 通道名称显示位置的y坐标 */
    //隐私遮挡
    DWORD  dwEnableHide;        /* 是否启动遮挡 ,0-否,1-是*/
    NET_DVR_SHELTER struShelter[MAX_SHELTERNUM];
    //OSD
    DWORD  dwShowOsd;// 预览的图象上是否显示OSD,0-不显示,1-显示
    WORD   wOSDTopLeftX;                /* OSD的x坐标 */
    WORD   wOSDTopLeftY;                /* OSD的y坐标 */
    BYTE    byOSDType;                    /* OSD类型(主要是年月日格式) */
    /* 0: XXXX-XX-XX 年月日 */
    /* 1: XX-XX-XXXX 月日年 */
    /* 2: XXXX年XX月XX日 */
    /* 3: XX月XX日XXXX年 */
    /* 4: XX-XX-XXXX 日月年*/
    /* 5: XX日XX月XXXX年 */
    /*6: xx/xx/xxxx(月/日/年) */
    /*7: xxxx/xx/xx(年/月/日) */
    /*8: xx/xx/xxxx(日/月/年)*/
    BYTE    byDispWeek;                /* 是否显示星期 */
    BYTE    byOSDAttrib;                /* OSD属性:透明，闪烁 */
    /* 0: 不显示OSD */
    /* 1: 透明，闪烁 */
    /* 2: 透明，不闪烁 */
    /* 3: 不透明，闪烁 */
    /* 4: 不透明，不闪烁 */    
    BYTE    byHourOSDType;                /* OSD小时制:0-24小时制,1-12小时制 */
    BYTE    byFontSize;      //16*16(中)/8*16(英)，1-32*32(中)/16*32(英)，2-64*64(中)/32*64(英)  3-48*48(中)/24*48(英) 4-24*24(中)/12*24(英) 5-96*96(中)/48*96(英) 6-128*128(中)/64*128(英) 7-80*80(中)/40*80(英) 8-112*112(中)/56*112(英) 0xff-自适应(adaptive)
    BYTE    byOSDColorType;     //0-默认（黑白）；1-自定义；2-勾边
    /*当对齐方式选择国标模式时，可以分别对右下角、左下角两个区域做字符叠加。
    右下角区域：
    共支持6行字符叠加，可以通过NET_DVR_SET_SHOWSTRING_V30/ NET_DVR_GET_SHOWSTRING_V30字符叠加接口，对应NET_DVR_SHOWSTRINGINFO结构体数组中的第0至第5个下标的值。叠加字符的方式为从下到上的方式。
    左下角区域：
    共支持2行字符叠加，可以通过NET_DVR_SET_SHOWSTRING_V3/ NET_DVR_GET_SHOWSTRING_V30字符叠加接口，对应NET_DVR_SHOWSTRINGINFO结构体数组中的第6和第7个下标的值。叠加字符的方式为从下到上的方式。
    */
    BYTE    byAlignment;//对齐方式 0-自适应，1-右对齐, 2-左对齐，3-国标模式，4-全部右对齐(包含叠加字符、时间以及标题等所有OSD字符)，5-全部左对齐(包含叠加字符、时间以及标题等所有OSD字符)
    BYTE    byOSDMilliSecondEnable;//视频叠加时间支持毫秒；0~不叠加, 1-叠加
    NET_DVR_VILOST_V40 struVILost;  //视频信号丢失报警（支持组）
    NET_DVR_VILOST_V40 struAULost;  /*音频信号丢失报警（支持组）*/
    NET_DVR_MOTION_V40 struMotion;  //移动侦测报警（支持组）
    NET_DVR_HIDEALARM_V40 struHideAlarm;  //遮挡报警（支持组）
    NET_DVR_RGB_COLOR    struOsdColor;//OSD颜色
    DWORD dwBoundary; //边界值，左对齐，右对齐以及国标模式的边界值，0-表示默认值，单位：像素;在国标模式下，单位修改为字符个数（范围是，0,1,2）
    NET_DVR_RGB_COLOR struOsdBkColor; //自定义OSD背景色
    BYTE    byOSDBkColorMode; //OSD背景色模式，0-默认，1-自定义OSD背景色
    BYTE    byUpDownBoundary; //上下最小边界值选项，单位为字符个数（范围是，0,1,2）,国标模式下无效。byAlignment=3该字段无效，通过dwBoundary进行边界配置，.byAlignment不等于3的情况下， byUpDownBoundary/byLeftRightBoundary配置成功后，dwBoundary值将不生效
    BYTE    byLeftRightBoundary; //左右最小边界值选项，单位为字符个数（范围是，0,1,2）, 国标模式下无效。byAlignment=3该字段无效，通过dwBoundary进行边界配置，.byAlignment不等于3的情况下， byUpDownBoundary/byLeftRightBoundary配置成功后，dwBoundary值将不生效
    BYTE    byAngleEnabled;//OSD是否叠加俯仰角信息,0~不叠加, 1-叠加
    WORD    wTiltAngleTopLeftX;    /* 俯仰角信息显示位置的x坐标 */
    WORD    wTiltAngleTopLeftY;  /* 俯仰角信息显示位置的y坐标 */
    BYTE    byRes[108];
}NET_DVR_PICCFG_V40,*LPNET_DVR_PICCFG_V40;


//sdk function
DWORD NET_DVR_GetSDKVersion();
DWORD NET_DVR_GetSDKBuildVersion();
int NET_DVR_IsSupport();

// sdk init
BOOL NET_DVR_Init();
BOOL NET_DVR_Cleanup();

//  login device
LONG NET_DVR_Login_V30(char *sDVRIP, WORD wDVRPort, char *sUserName, char *sPassword, LPNET_DVR_DEVICEINFO_V30 lpDeviceInfo);
LONG NET_DVR_Login_V40(LPNET_DVR_USER_LOGIN_INFO pLoginInfo,LPNET_DVR_DEVICEINFO_V40 lpDeviceInfo);
BOOL NET_DVR_Logout(LONG lUserID);
BOOL NET_DVR_Logout_V30(LONG lUserID);

// connect device
BOOL NET_DVR_SetConnectTime(DWORD dwWaitTime, DWORD dwTryTimes);
BOOL NET_DVR_SetReconnect(DWORD dwInterval, BOOL bEnableRecon);

// sdk capture
BOOL NET_DVR_CaptureJPEGPicture(LONG lUserID, LONG lChannel, LPNET_DVR_JPEGPARA lpJpegPara, char *sPicFileName);
BOOL NET_DVR_CaptureJPEGPicture(LONG lUserID, LONG lChannel, LPNET_DVR_JPEGPARA lpJpegPara, char *sPicFileName);

// stream play control
LONG NET_DVR_RealPlay_V30(LONG lUserID, LPNET_DVR_CLIENTINFO lpClientInfo, void(*fRealDataCallBack_V30) (LONG lRealHandle, DWORD dwDataType, BYTE *pBuffer, DWORD dwBufSize, void* pUser), void* pUser, BOOL bBlocked);
BOOL NET_DVR_ClosePreview(LONG lUserID, DWORD nSessionID);
BOOL NET_DVR_ClosePlayBack(LONG lUserID, DWORD nSessionID);
LONG NET_DVR_RealPlay_V40(LONG lUserID, LPNET_DVR_PREVIEWINFO lpPreviewInfo, REALDATACALLBACK fRealDataCallBack_V30, void* pUser);
BOOL NET_DVR_StopRealPlay(LONG lRealHandle);

BOOL NET_DVR_SaveRealData(LONG lRealHandle,char *sFileName);
BOOL NET_DVR_StopSaveRealData(LONG lRealHandle);

// device ptz control
BOOL NET_DVR_PTZControlWithSpeed(LONG lRealHandle, DWORD dwPTZCommand, DWORD dwStop, DWORD dwSpeed);
BOOL NET_DVR_PTZControlWithSpeed_Other(LONG lUserID, LONG lChannel, DWORD dwPTZCommand, DWORD dwStop, DWORD dwSpeed);
    //云台控制相关接口
BOOL NET_DVR_PTZControl(LONG lRealHandle, DWORD dwPTZCommand, DWORD dwStop);

BOOL NET_DVR_PTZControl_Other(LONG lUserID, LONG lChannel, DWORD dwPTZCommand, DWORD dwStop);

DWORD NET_DVR_GetLastError();

// alarm
BOOL NET_DVR_GetDeviceAbility(LONG lUserID, DWORD dwAbilityType, char* pInBuf, DWORD dwInLength, char* pOutBuf, DWORD dwOutLength);

BOOL NET_DVR_GetDVRConfig(LONG lUserID, DWORD dwCommand,LONG lChannel, LPVOID lpOutBuffer, DWORD dwOutBufferSize, LPDWORD lpBytesReturned);
BOOL NET_DVR_SetDVRConfig(LONG lUserID, DWORD dwCommand,LONG lChannel, LPVOID lpInBuffer, DWORD dwInBufferSize);

BOOL NET_DVR_GetDeviceConfig(LONG lUserID, DWORD dwCommand, DWORD dwCount, LPVOID lpInBuffer, DWORD dwInBufferSize, LPVOID lpStatusList, LPVOID lpOutBuffer, DWORD dwOutBufferSize);
BOOL NET_DVR_SetDeviceConfigEx(LONG lUserID, DWORD dwCommand, DWORD dwCount, NET_DVR_IN_PARAM *lpInParam, NET_DVR_OUT_PARAM *lpOutParam);

// 报警消息回调
typedef void (CALLBACK *MSGCallBack)(LONG lCommand, NET_DVR_ALARMER *pAlarmer, char *pAlarmInfo, DWORD dwBufLen, void* pUser);
BOOL NET_DVR_SetDVRMessageCallBack_V30(MSGCallBack fMessageCallBack, void* pUser);
typedef BOOL (CALLBACK *MSGCallBack_V31)(LONG lCommand, NET_DVR_ALARMER *pAlarmer, char *pAlarmInfo, DWORD dwBufLen, void* pUser);
BOOL NET_DVR_SetDVRMessageCallBack_V31(MSGCallBack_V31 fMessageCallBack, void* pUser);
BOOL NET_DVR_SetDVRMessageCallBack_V50(int iIndex, MSGCallBack fMessageCallBack, void* pUser);
BOOL NET_DVR_SetDVRMessageCallBack_V51(int iIndex, MSGCallBack fMsgCallBack, void* pUser);


// 报警消息监听
BOOL NET_DVR_StartListen(char *sLocalIP,WORD wLocalPort);
BOOL NET_DVR_StopListen();

LONG NET_DVR_StartListen_V30(char *sLocalIP, WORD wLocalPort, MSGCallBack DataCallback, void* pUserData);
BOOL NET_DVR_StopListen_V30(LONG lListenHandle);
BOOL NET_DVR_ContinuousShoot(LONG lUserID, LPNET_DVR_SNAPCFG lpInter);

//报警
LONG NET_DVR_SetupAlarmChan(LONG lUserID);
BOOL NET_DVR_CloseAlarmChan(LONG lAlarmHandle);
LONG NET_DVR_SetupAlarmChan_V30(LONG lUserID);
BOOL NET_DVR_CloseAlarmChan_V30(LONG lAlarmHandle);
LONG NET_DVR_SetupAlarmChan_V41(LONG lUserID, LPNET_DVR_SETUPALARM_PARAM lpSetupParam);


// car capture

//BOOL NET_DVR_ManualSnap(LONG lUserID, NET_DVR_MANUALSNAP const* lpInter, LPNET_DVR_PLATE_RESULT lpOuter);
BOOL NET_DVR_ContinuousShoot(LONG lUserID, LPNET_DVR_SNAPCFG lpInter);

#endif