# MP4 Box Structure

| Col1 | Col2 | Col3 | Col4 | Col5 | Col6 | Required | Description |
|------|------|------|------|------|------|----------|-------------|
| ftyp | | | | | | ✓ | file type and compatibility<br>文件类型和兼容性 |
| pdin | | | | | | | progressive download information |
| moov | | | | | | ✓ | container for all the metadata<br>所有元数据的容器 |
| | mvhd | | | | | ✓ | movie header, overall declarations<br>电影头，整体声明 |
| | trak | | | | | ✓ | container for an individual track or stream<br>单个轨或流的容器 |
| | | tkhd | | | | ✓ | track header, overall information about the track<br>轨的头部，关于该轨的概括信息，比如视频宽高 |
| | | tref | | | | | track reference container |
| | | edts | | | | | edit list container |
| | | | elst | | | | an edit list |
| | | mdia | | | | ✓ | container for the media information in a track<br>轨媒体信息的容器 |
| | | | mdhd | | | ✓ | media header, overall information about the media<br>媒体头，关于媒体的总体信息 |
| | | | hdlr | | | ✓ | handler, declares the media (handler) type<br>媒体的播放过程信息 |
| | | | minf | | | ✓ | media information container<br>媒体信息容器 |
| | | | | vmhd | | | video media header, overall information (video track only) |
| | | | | hmhd | | | hint media header, overall information (hint track only) |
| | | | | nmhd | | | Null media header, overall information (some tracks only) |
| | | | | dinf | | ✓ | data information box, container<br>数据信息box，容器 |
| | | | | | dref | ✓ | data reference box, declares source(s) of media data in track<br>如何定位媒体信息 |
| | | | | stbl | | ✓ | sample table box, container for the time/space map<br>包含了track中的sample的所有时间和位置信息，以及sample的编解码等信息 |
| | | | | | stsd | ✓ | sample descriptions (codec types, initialization etc.)<br>如果是视频，包含：编码类型、宽高、长度等信息；<br>如果是音频，包含：声道、采样率等信息 |
| | | | | | stts | ✓ | (decoding) time-to-sample<br>描述了sample时序的映射方法，我们可以通过它找到任何时间的sample |
| | | | | | ctts | | (composition) time to sample |
| | | | | | stsc | ✓ | sample-to-chunk, partial data-offset information<br>用chunk组织sample可以方便优化数据获取，一个chunk包含一个或多个sample |
| | | | | | stsz | | sample sizes (framing)<br>每个sample的大小<br>虽然这里没有打勾，但对于mp4还是非常必要的 |
| | | | | | stz2 | | compact sample sizes (framing) |
| | | | | | stco | ✓ | chunk offset, partial data-offset information<br>定义了每个chunk在媒体流中的偏移位置 |
| | | | | | co64 | | 64-bit chunk offset |
| | | | | | stss | | sync sample table (random access points)<br>用于确定media中的关键帧 |
| | | | | | stsh | | shadow sync sample table |
| | | | | | padb | | sample padding bits |
| | | | | | stdp | | sample degradation priority |
| | | | | | sdtp | | independent and disposable samples |
| | | | | | sbgp | | sample-to-group |
| | | | | | sgpd | | sample group description |
| | | | | | subs | | sub-sample information |
| | mvex | | | | | | movie extends box |
| | | mehd | | | | | movie extends header box |
| | | trex | | | | ✓ | track extends defaults |
| | ipmc | | | | | | IPMP Control Box |
| moof | | | | | | | movie fragment |
| | mfhd | | | | | ✓ | movie fragment header |
| | traf | | | | | | track fragment |
| | | | tfhd | | | ✓ | track fragment header |
| | | | trun | | | | track fragment run |
| | | | sdtp | | | | independent and disposable samples |
| | | | sbgp | | | | sample-to-group subs sub-sample information |
| mfra | | | | | | | movie fragment random access |
| | tfra | | | | | | track fragment random access |
| | mfro | | | | | ✓ | movie fragment random access offset |
| mdat | | | | | | | media data container |
| free | | | | | | | free space |
| skip | | | | | | | free space |
| | udta | | | | | | user-data |
| | | cprt | | | | | copyright etc. |
| meta | | | | | | | metadata |
| | hdlr | | | | | ✓ | handler, declares the metadata (handler) type |
| | dinf | | | | | | data information box, container |
| | | dref | | | | | data reference box, declares source(s) of metadata items |
| | ipmc | | | | | | IPMP Control Box |
| | iloc | | | | | | item location |
| | ipro | | | | | | item protection |
| | | sinf | | | | | protection scheme information box |
| | | | frma | | | | original format box |
| | | | imif | | | | IPMP Information box |
| | | | schm | | | | scheme type box |
| | | | schi | | | | scheme information box |
| | iinf | | | | | | item information |
| | xml | | | | | | XML container |
| | bxml | | | | | | binary XML container |
| | pitm | | | | | | primary item reference |
| fiin | | | | | | | file delivery item information |
| | | paen | | | | | partition entry |
| | | | fpar | | | | file partition |
| | | | | fecr | | | FEC reservoir |
| | | | segr | | | | file delivery session group |
| | | | gitn | | | | group id to name |
| | | | tsel | | | | track selection |
| meco | | | | | | | additional metadata container |
| | mere | | | | | | metabox relation |

> Source: [MP4格式分析](https://blog.csdn.net/weixin_41643938/article/details/124542849) 