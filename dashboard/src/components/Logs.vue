<template>
  <div style="padding:0 15px">
    <div>
      自动滚动
      <Switch v-model="autoScroll" />
    </div>
    <div ref="logContainer" class="log-container">
      <pre><template v-for="item in $store.state.logs">{{item+"\n"}}</template></pre>
    </div>
  </div>
</template>

<script>
import { mapActions } from "vuex";
export default {
  data() {
    return {
      autoScroll: true
    };
  },
  mounted() {
    this.fetchLogs();
  },
  destroyed() {
    this.stopFetchLogs();
  },
  methods: {
    ...mapActions(["fetchLogs", "stopFetchLogs"])
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
  max-height: 500px;
}
</style>