<template>
  <div style="text-align:left;">
    <Tabs v-model="currentTab" @on-click="onChangeTab">
      <TabPane label="直播流" icon="md-videocam">
        <div class="layout">
          <Card v-for="item in Rooms" :key="item.StreamPath" class="room">
            <p slot="title">{{typeMap[item.Type]||item.Type}}{{item.StreamPath}}</p>
            <StartTime slot="extra" :value="item.StartTime"></StartTime>
            <p>
              {{SoundFormat(item.AudioInfo.SoundFormat)}} {{item.AudioInfo.PacketCount}}
              {{SoundRate(item.AudioInfo.SoundRate)}} 声道:{{item.AudioInfo.SoundType}}
            </p>
            <p>
              {{CodecID(item.VideoInfo.CodecID)}} {{item.VideoInfo.PacketCount}}
              {{item.VideoInfo.SPSInfo.Width}}x{{item.VideoInfo.SPSInfo.Height}}
            </p>
            <ButtonGroup size="small">
              <Button @click="onShowDetail(item)" icon="ios-people">{{getSubscriberCount(item)}}</Button>
              <Button v-if="item.Type" @click="preview(item)" icon="md-eye"></Button>
              <Button
                @click="stopRecord(item)"
                class="recording"
                v-if="isRecording(item)"
                icon="ios-radio-button-on"
              ></Button>
              <Button @click="record(item)" v-else icon="ios-radio-button-on"></Button>
            </ButtonGroup>
          </Card>
          <div v-if="Rooms.length==0" class="empty">
            <Icon type="md-wine" size="50" />没有任何房间
          </div>
        </div>
      </TabPane>
      <TabPane label="集群总览" icon="ios-cloud">
        <Cluster />
      </TabPane>
      <TabPane label="录制的视频" icon="ios-folder" name="recordsPanel">
        <Records ref="recordsPanel" />
      </TabPane>
      <TabPane label="日志跟踪" icon="md-bug">
        <Logs />
      </TabPane>
      <TabPane label="查看配置" icon="md-settings" name="configPanel">
        <Config ref="configPanel" />
      </TabPane>
    </Tabs>
    <div class="status">
      <Alert>带宽消耗 📥：{{totalInNetSpeed}} 📤：{{totalOutNetSpeed}}</Alert>
      <Alert
        :type="memoryStatus"
      >内存使用：{{networkFormat(Memory.Used,"M")}} 占比：{{Memory.Usage.toFixed(2)}}%</Alert>
      <Alert :type="cpuStatus">CPU使用：{{CPUUsage.toFixed(2)}}%</Alert>
      <Alert
        :type="hardDiskStatus"
      >磁盘使用：{{networkFormat(HardDisk.Used,"M")}} 占比：{{HardDisk.Usage.toFixed(2)}}%</Alert>
    </div>
    <Jessibuca ref="jessibuca" v-model="showPreview" :videoCodec="currentStream && CodecID(currentStream.VideoInfo.CodecID)" :audioCodec="currentStream && SoundFormat(currentStream.AudioInfo.SoundFormat)"></Jessibuca>
    <Subscribers :data="currentStream && currentStream.SubscriberInfo || []" v-model="showSubscribers" />
  </div>
</template>

<script>
import { mapActions, mapState } from "vuex";
import Jessibuca from "../components/Jessibuca";
import StartTime from "../components/StartTime";
import Records from "../components/Records";
import Logs from "../components/Logs";
import Config from "../components/Config";
import Subscribers from "../components/Subscribers";
import Cluster from "../components/Cluster";
const uintInc = {
  "": "K",
  K: "M",
  M: "G",
  G: null
};
const SoundFormat = {
  0: "Linear PCM, platform endian",
  1: "ADPCM",
  2: "MP3",
  3: "Linear PCM, little endian",
  4: "Nellymoser 16kHz mono",
  5: "Nellymoser 8kHz mono",
  6: "Nellymoser",
  7: "G.711 A-law logarithmic PCM",
  8: "G.711 mu-law logarithmic PCM",
  9: "reserved",
  10: "AAC",
  11: "Speex",
  14: "MP3 8Khz",
  15: "Device-specific sound"
};
const CodecID = {
  1: "JPEG (currently unused)",
  2: "Sorenson H.263",
  3: "Screen video",
  4: "On2 VP6",
  5: "On2 VP6 with alpha channel",
  6: "Screen video version 2",
  7: "AVC",
  12: "H265"
};
export default {
  name: "Console",
  components: {
    Jessibuca,
    StartTime,
    Records,
    Logs,
    Subscribers,
    Config,
    Cluster
  },
  data() {
    return {
      showPreview: false,
      showSubscribers: false,
      currentTab: "",
      currentStream: null,
      typeMap: {
        Receiver: "📡",
        FlvFile: "🎥",
        TS: "🎬",
        HLS: "🍎",
        "": "⏳",
        Match365: "🏆",
        RTMP: "🚠"
      }
    };
  },
  computed: {
    ...mapState({
      Rooms: state => state.summary.Rooms || [],
      Memory: state => state.summary.Memory,
      CPUUsage: state => state.summary.CPUUsage,
      HardDisk: state => state.summary.HardDisk,
      cpuStatus: state => {
        if (state.summary.CPUUsage > 99) return "error";
        return state.summary.CPUUsage > 50 ? "warning" : "success";
      },
      memoryStatus(state) {
        if (state.summary.CPUUsage > 99) return "error";
        return state.summary.CPUUsage > 50 ? "warning" : "success";
      },
      hardDiskStatus(state) {
        if (state.summary.CPUUsage > 99) return "error";
        return state.summary.CPUUsage > 50 ? "warning" : "success";
      },
      totalInNetSpeed(state) {
        return (
          this.networkFormat(
            state.summary.NetWork
              ? state.summary.NetWork.reduce(
                  (aac, c) => aac + c.ReceiveSpeed,
                  0
                )
              : 0
          ) + "/S"
        );
      },
      totalOutNetSpeed(state) {
        return (
          this.networkFormat(
            state.summary.NetWork
              ? state.summary.NetWork.reduce((aac, c) => aac + c.SentSpeed, 0)
              : 0
          ) + "/S"
        );
      }
    })
  },
  methods: {
    ...mapActions(["fetchSummary", "stopFetchSummary"]),
    getSubscriberCount(item) {
      if (
        this.currentStream &&
        this.currentStream.StreamPath == item.StreamPath
      ) {
        this.currentStream = item;
      }
      return item.SubscriberInfo ? item.SubscriberInfo.length : 0;
    },
    preview(item) {
      this.currentStream = item;
      this.$nextTick(() =>
        this.$refs.jessibuca.play(
          "ws://" + location.hostname + ":8080/" + item.StreamPath
        )
      );
      this.showPreview = true;
    },
    onShowDetail(item) {
      this.showSubscribers = true;
      this.currentStream = item;
    },
    networkFormat(value, unit = "") {
      if (value > 1024 && uintInc[unit]) {
        return this.networkFormat(value / 1024, uintInc[unit]);
      }
      return value.toFixed(2).replace(".00", "") + unit + "B";
    },
    SoundFormat(soundFormat) {
      return SoundFormat[soundFormat];
    },
    CodecID(codec) {
      return CodecID[codec];
    },
    SoundRate(rate) {
      return rate > 1000 ? rate / 1000 + "kHz" : rate + "Hz";
    },
    record(item) {
      this.$Modal.confirm({
        title: "提示",
        content: "<p>是否使用追加模式</p><small>选择取消将覆盖已有文件</small>",
        onOk: () => {
          window.ajax.get(
            "//" + location.host + "/api/record/flv?append=true",
            { streamPath: item.StreamPath },
            x => {
              if (x == "success") {
                this.$Message.success("开始录制(追加模式)");
              } else {
                this.$Message.error(x);
              }
            }
          );
        },
        onCancel: () => {
          window.ajax.get(
            "//" + location.host + "/api/record/flv",
            { streamPath: item.StreamPath },
            x => {
              if (x == "success") {
                this.$Message.success("开始录制");
              } else {
                this.$Message.error(x);
              }
            }
          );
        }
      });
    },
    stopRecord(item) {
      window.ajax.get(
        "//" + location.host + "/api/record/flv/stop",
        { streamPath: item.StreamPath },
        x => {
          if (x == "success") {
            this.$Message.success("停止录制");
          } else {
            this.$Message.error(x);
          }
        }
      );
    },
    isRecording(item) {
      return (
        item.SubscriberInfo &&
        item.SubscriberInfo.find(x => x.Type == "FlvRecord")
      );
    },
    onChangeTab(name) {
      switch (name) {
        case "recordsPanel":
          this.$refs.recordsPanel.onVisible(true);
          break;
        case "configPanel":
          this.$refs.configPanel.onVisible(true);
      }
    }
  },
  mounted() {
    this.fetchSummary();
  },
  destroyed() {
    this.stopFetchSummary();
  }
};
</script>

<style scoped>
@keyframes recording {
  0% {
    opacity: 0.2;
  }
  50% {
    opacity: 1;
  }
  100% {
    opacity: 0.2;
  }
}

.recording {
  animation: recording 1s infinite;
}

.layout {
  padding-bottom: 30px;
  display: flex;
  flex-wrap: wrap;
}

.room {
  width: 250px;
  margin: 10px;
  text-align: left;
}

.empty {
  color: #eb5e46;
  width: 100%;
  min-height: 500px;
  display: flex;
  justify-content: center;
  align-items: center;
}

.status {
  position: fixed;
  display: flex;
  left: 5px;
  bottom: 10px;
}

.status > div {
  margin: 0 5px;
}
</style>