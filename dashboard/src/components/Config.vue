<template>
  <div style="padding:0 15px">
    <pre>{{config}}</pre>
  </div>
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