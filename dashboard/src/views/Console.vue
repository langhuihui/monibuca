<template>
    <div class="layout">
        <ButtonGroup vertical>
            <Button icon="ios-folder"></Button>
            <Button icon="md-bug"></Button>
            <Button icon="md-settings"></Button>
        </ButtonGroup>
        <Card v-for="item in Rooms" :key="item.StreamPath" class="room">
            <p slot="title">
                {{typeMap[item.Type]||item.Type}}{{item.StreamPath}}
            </p>
            <StartTime slot="extra" :value="item.StartTime"></StartTime>
            <p>
                {{SoundFormat(item.AudioInfo.SoundFormat)}} {{item.AudioInfo.PacketCount}}
                {{SoundRate(item.AudioInfo.SoundRate)}} 声道:{{item.AudioInfo.SoundType}}
            </p>
            <p>
                {{CodecID(item.VideoInfo.CodecID)}} {{item.VideoInfo.PacketCount}}
                {{item.VideoInfo.SPSInfo.Width}}x{{item.VideoInfo.SPSInfo.Height}}
            </p>
            <Button @click="onShowDetail(item)">
                <Icon type="ios-people"/>
                {{item.SubscriberInfo?item.SubscriberInfo.length:0}}
            </Button>
            <Button v-if="item.Type" @click="preview(item)">
                <Icon type="md-eye"/>
                Preview
            </Button>
            <Button @click="stopRecord(item)" v-if="isRecording(item)">
                <Icon type="ios-radio-button-on" class="recording"/>
                Stop Rec
            </Button>
            <Button @click="record(item)" v-else>
                <Icon type="ios-radio-button-on"/>
                Rec
            </Button>
        </Card>
        <div v-if="Rooms.length==0" class="empty">
            <Icon type="md-wine" size="50"/>
            没有任何房间
        </div>
        <div class="status">
            <Alert>
                带宽消耗 📥：{{totalInNetSpeed}} 📤：{{totalOutNetSpeed}}
            </Alert>
            <Alert :type="memoryStatus">
                内存使用：{{networkFormat(Memory.Used,"M")}} 占比：{{Memory.Usage.toFixed(2)}}%
            </Alert>
            <Alert :type="cpuStatus">
                CPU使用：{{CPUUsage.toFixed(2)}}%
            </Alert>
            <Alert :type="hardDiskStatus">
                磁盘使用：{{networkFormat(HardDisk.Used,"M")}} 占比：{{HardDisk.Usage.toFixed(2)}}%
            </Alert>
        </div>
        <Jessibuca ref="jessibuca" v-model="showPreview"></Jessibuca>
    </div>
</template>

<script>
    import {mapActions, mapState} from 'vuex'
    import Jessibuca from "../components/Jessibuca";
    import StartTime from "../components/StartTime";

    const uintInc = {
        "": "K",
        K: "M",
        M: "G",
        G: null
    }
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
    }
    const CodecID = {
        1: "JPEG (currently unused)",
        2: "Sorenson H.263",
        3: "Screen video",
        4: "On2 VP6",
        5: "On2 VP6 with alpha channel",
        6: "Screen video version 2",
        7: "AVC",
        12: "H265"
    }
    export default {
        name: "Console",
        components: {
            Jessibuca, StartTime
        },
        data() {
            return {
                showPreview: false,
                typeMap: {
                    TS: "🎬", HLS: "🍎", "": "⏳", Match365: "🏆", RTMP: "📸"
                }
            }
        },
        computed: {
            ...mapState({
                Rooms: state => state.summary.Rooms || [],
                Memory: state => state.summary.Memory,
                CPUUsage: state => state.summary.CPUUsage,
                HardDisk: state => state.summary.HardDisk,
                cpuStatus: state => {
                    if (state.summary.CPUUsage > 99)
                        return "error"
                    return state.summary.CPUUsage > 50 ? "warning" : "success"
                },
                memoryStatus(state) {
                    if (state.summary.CPUUsage > 99)
                        return "error"
                    return state.summary.CPUUsage > 50 ? "warning" : "success"
                },
                hardDiskStatus(state) {
                    if (state.summary.CPUUsage > 99)
                        return "error"
                    return state.summary.CPUUsage > 50 ? "warning" : "success"
                },
                totalInNetSpeed(state) {
                    return this.networkFormat(state.summary.NetWork.reduce((aac, c) => aac + c.ReceiveSpeed, 0)) + "/S"
                },
                totalOutNetSpeed(state) {
                    return this.networkFormat(state.summary.NetWork.reduce((aac, c) => aac + c.SentSpeed, 0)) + "/S"
                }
            }),
        },
        methods: {
            ...mapActions([
                'fetchSummary',
                'stopFetchSummary'
            ]),
            preview(item) {
                this.$refs.jessibuca.play("ws://" + location.hostname + ":8080/" + item.StreamPath)
                this.showPreview = true
            }, onShowDetail() {
                // this.showDetail = true
                // this.currentSub = item
            },
            networkFormat(value, unit = "") {
                if (value > 1024 && uintInc[unit]) {
                    return this.networkFormat(value / 1024, uintInc[unit])
                }
                return value.toFixed(2).replace(".00", "") + unit + "B"
            },
            SoundFormat(soundFormat) {
                return SoundFormat[soundFormat]
            },
            CodecID(codec) {
                return CodecID[codec]
            },
            SoundRate(rate) {
                return rate > 1000 ? (rate / 1000) + "kHz" : rate + "Hz"
            },
            record(item) {
                ajax.get("//" + location.host + "/api/record/flv",{streamPath:item.StreamPath})
            },
            stopRecord(item){
                ajax.get("//" + location.host + "/api/record/flv/stop",{streamPath:item.StreamPath})
            },
            isRecording(item) {
                return item.SubscriberInfo && item.SubscriberInfo.find(x => x.Type == "FlvRecord")
            }
        },
        mounted() {
            this.fetchSummary()
        },
        destroyed() {
            this.stopFetchSummary()
        }
    }
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
        position: relative;
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