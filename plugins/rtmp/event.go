package rtmp

// http://help.adobe.com/zh_CN/AIR/1.5/jslr/flash/events/NetStatusEvent.html

const (
	Response_OnStatus = "onStatus"
	Response_Result   = "_result"
	Response_Error    = "_error"

	/* Level */
	Level_Status  = "status"
	Level_Error   = "error"
	Level_Warning = "warning"

	/* Code */
	/* NetStream */
	NetStream_Play_Reset          = "NetStream.Play.Reset"          // "status" 由播放列表重置导致
	NetStream_Play_Start          = "NetStream.Play.Start"          // "status" 播放已开始
	NetStream_Play_StreamNotFound = "NetStream.Play.StreamNotFound" // "error"  无法找到传递给 play()方法的 FLV
	NetStream_Play_Stop           = "NetStream.Play.Stop"           // "status" 播放已结束
	NetStream_Play_Failed         = "NetStream.Play.Failed"         // "error"  出于此表中列出的原因之外的某一原因(例如订阅者没有读取权限),播放发生了错误

	NetStream_Play_Switch   = "NetStream.Play.Switch"
	NetStream_Play_Complete = "NetStream.Play.Switch"

	NetStream_Data_Start = "NetStream.Data.Start"

	NetStream_Publish_Start     = "NetStream.Publish.Start"     // "status"	已经成功发布.
	NetStream_Publish_BadName   = "NetStream.Publish.BadName"   // "error"	试图发布已经被他人发布的流.
	NetStream_Publish_Idle      = "NetStream.Publish.Idle"      // "status"	流发布者空闲而没有在传输数据.
	NetStream_Unpublish_Success = "NetStream.Unpublish.Success" // "status"	已成功执行取消发布操作.

	NetStream_Buffer_Empty   = "NetStream.Buffer.Empty"   // "status" 数据的接收速度不足以填充缓冲区.数据流将在缓冲区重新填充前中断,此时将发送 NetStream.Buffer.Full 消息,并且该流将重新开始播放
	NetStream_Buffer_Full    = "NetStream.Buffer.Full"    // "status" 缓冲区已满并且流将开始播放
	NetStream_Buffe_Flush    = "NetStream.Buffer.Flush"   // "status" 数据已完成流式处理,剩余的缓冲区将被清空
	NetStream_Pause_Notify   = "NetStream.Pause.Notify"   // "status" 流已暂停
	NetStream_Unpause_Notify = "NetStream.Unpause.Notify" // "status" 流已恢复

	NetStream_Record_Start    = "NetStream.Record.Start"    // "status"	录制已开始.
	NetStream_Record_NoAccess = "NetStream.Record.NoAccess" // "error"	试图录制仍处于播放状态的流或客户端没有访问权限的流.
	NetStream_Record_Stop     = "NetStream.Record.Stop"     // "status"	录制已停止.
	NetStream_Record_Failed   = "NetStream.Record.Failed"   // "error"	尝试录制流失败.

	NetStream_Seek_Failed      = "NetStream.Seek.Failed"      // "error"	搜索失败,如果流处于不可搜索状态,则会发生搜索失败.
	NetStream_Seek_InvalidTime = "NetStream.Seek.InvalidTime" // "error"	对于使用渐进式下载方式下载的视频,用户已尝试跳过到目前为止已下载的视频数据的结尾或在整个文件已下载后跳过视频的结尾进行搜寻或播放. message.details 属性包含一个时间代码,该代码指出用户可以搜寻的最后一个有效位置.
	NetStream_Seek_Notify      = "NetStream.Seek.Notify"      // "status"	搜寻操作完成.

	/* NetConnect */
	NetConnection_Call_BadVersion     = "NetConnection.Call.BadVersion"     // "error"	以不能识别的格式编码的数据包.
	NetConnection_Call_Failed         = "NetConnection.Call.Failed"         // "error"	NetConnection.call 方法无法调用服务器端的方法或命令.
	NetConnection_Call_Prohibited     = "NetConnection.Call.Prohibited"     // "error"	Action Message Format (AMF) 操作因安全原因而被阻止. 或者是 AMF URL 与 SWF 不在同一个域,或者是 AMF 服务器没有信任 SWF 文件的域的策略文件.
	NetConnection_Connect_AppShutdown = "NetConnection.Connect.AppShutdown" // "error"	正在关闭指定的应用程序.
	NetConnection_Connect_InvalidApp  = "NetConnection.Connect.InvalidApp"  // "error"	连接时指定的应用程序名无效.
	NetConnection_Connect_Success     = "NetConnection.Connect.Success"     // "status"	连接尝试成功.
	NetConnection_Connect_Closed      = "NetConnection.Connect.Closed"      // "status"	成功关闭连接.
	NetConnection_Connect_Failed      = "NetConnection.Connect.Failed"      // "error"	连接尝试失败.
	NetConnection_Connect_Rejected    = "NetConnection.Connect.Rejected"    // "error"  连接尝试没有访问应用程序的权限.

	/* SharedObject */
	SharedObject_Flush_Success  = "SharedObject.Flush.Success"  //"status"	"待定"状态已解析并且 SharedObject.flush() 调用成功.
	SharedObject_Flush_Failed   = "SharedObject.Flush.Failed"   //"error"	"待定"状态已解析,但 SharedObject.flush() 失败.
	SharedObject_BadPersistence = "SharedObject.BadPersistence" //"error"	使用永久性标志对共享对象进行了请求,但请求无法被批准,因为已经使用其它标记创建了该对象.
	SharedObject_UriMismatch    = "SharedObject.UriMismatch"    //"error"	试图连接到拥有与共享对象不同的 URI (URL) 的 NetConnection 对象.
)

// NetConnection、NetStream 或 SharedObject 对象报告其状态时,将调度 NetStatusEvent 对象

type NetStatusEvent struct {
	Code  string
	Level string
}

func newNetStatusEvent(code, level string) (e *NetStatusEvent) {
	e.Code = code
	e.Level = level
	return e
}
