<template>
  <Modal
    v-bind="$attrs"
    draggable
    v-on="$listeners"
    :title="url"
    @on-ok="onClosePreview"
    @on-cancel="onClosePreview"
  >
    <canvas id="canvas" width="488" height="275" style="background: black" />
    <div slot="footer">
      <!--      音频缓冲：-->
      <!--      <InputNumber v-model="audioBuffer" size="small"></InputNumber>-->
      <Button v-if="audioEnabled" @click="turnOff" icon="md-volume-off" />
      <Button v-else @click="turnOn" icon="md-volume-up"></Button>
    </div>
  </Modal>
</template>

<script>
let h5lc = null;
export default {
  name: "Jessibuca",
  props: {
    audioCodec: String,
    videoCodec: String
  },
  data() {
    return {
      audioEnabled: false,
      url: "",
      decoderTable: {
        AAC_AVC: "ff",
        AAC_H265: "hevc_aac",
        MP3_AVC: "ff_mp3",
        MP3_H265: "hevc_mp3"
      }
    };
  },
  watch: {
    audioEnabled(value) {
      h5lc.audioEnabled(value);
    },
    decoder(value) {
      if (h5lc) {
        h5lc.destroy();
      }
      h5lc = new window.Jessibuca({
        canvas: document.getElementById("canvas"),
        decoder: value
      });
    }
  },
  computed: {
    decoder() {
      return (
        "jessibuca/" +
        this.decoderTable[this.audioCodec + "_" + this.videoCodec] +
        ".js"
      );
    }
  },
  mounted() {
    h5lc = new window.Jessibuca({
      canvas: document.getElementById("canvas"),
      decoder: "jessibuca/ff.js"
    });
  },
  destroyed() {
    this.onClosePreview();
    h5lc.destroy();
  },
  methods: {
    play(url) {
      this.url = url;
      h5lc.play(url);
    },
    onClosePreview() {
      h5lc.close();
    },
    turnOn() {
      this.audioEnabled = true;
    },
    turnOff() {
      this.audioEnabled = false;
    }
  }
};
</script>

