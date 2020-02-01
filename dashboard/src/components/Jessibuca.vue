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
      <Button v-if="audioEnabled" @click="turnOff" icon="md-volume-off" />
      <Button v-else @click="turnOn" icon="md-volume-up"></Button>
    </div>
  </Modal>
</template>

<script>
let h5lc = null;
export default {
  name: "Jessibuca",
  data() {
    return {
      audioEnabled: false,
      url: ""
    };
  },
  watch: {
    audioEnabled(value) {
      h5lc.audioEnabled(value);
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

