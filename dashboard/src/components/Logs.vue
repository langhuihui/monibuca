<template>
  <Modal v-bind="$attrs" draggable v-on="$listeners" @on-visible-change="onVisible" title="日志跟踪">
    <div ref="logContainer" class="log-container">
      <pre><template v-for="item in $store.state.logs">{{item+"\n"}}</template></pre>
    </div>
    <div slot="footer">
      自动滚动
      <Switch v-model="autoScroll" />
    </div>
  </Modal>
</template>

<script>
import { mapActions } from "vuex";
export default {
  data() {
    return {
      autoScroll: true
    };
  },
  methods: {
    ...mapActions(["fetchLogs", "stopFetchLogs"]),
    onVisible(visible) {
      if (visible) {
        this.fetchLogs();
      } else {
        this.stopFetchLogs();
      }
    }
  },
  updated() {
    if (this.autoScroll) {
      this.$refs.logContainer.scrollTop = this.$refs.logContainer.offsetHeight;
    }
  }
};
</script>

<style>
.log-container {
  overflow-y: auto;
  max-height: 360px;
}
</style>