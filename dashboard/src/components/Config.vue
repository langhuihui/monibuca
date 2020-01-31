<template>
  <Modal v-bind="$attrs" draggable v-on="$listeners" @on-visible-change="onVisible" title="配置">
    <div>
      <pre>{{config}}</pre>
    </div>
  </Modal>
</template>

<script>
export default {
  data() {
    return {
      config: ""
    };
  },
  methods: {
    onVisible(visible) {
      if (visible) {
        window.ajax.get(
          "//" + location.host + "/api/config",
          {},
          x => (this.config = x)
        );
      }
    }
  }
};
</script>

<style>
</style>