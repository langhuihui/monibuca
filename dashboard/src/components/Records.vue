<template>
  <div class="records">
    <Card v-for="item in data" :key="item">
      <p slot="title">{{item.Path}}</p>
      <div slot="extra">
        <Button @click="play(item)" icon="md-play" size="small"></Button>
        <Button @click="deleteFlv(item)" icon="ios-trash" size="small"></Button>
      </div>
      {{toSizeStr(item.Size)}} {{toDurationStr(item.Duration)}}
    </Card>
  </div>
</template>

<script>
const uintInc = {
  "": "K",
  K: "M",
  M: "G",
  G: null
};

export default {
  data() {
    return {
      data: []
    };
  },
  methods: {
    play(item) {
      window.ajax.get(
        "//" + location.host + "/api/record/flv/play",
        { streamPath: item.Path.replace(".flv", "") },
        x => {
          if (x == "success") {
            this.onVisible(true);
            this.$Message.success("删除成功");
          } else {
            this.$Message.error(x);
          }
        }
      );
    },
    deleteFlv(item) {
      this.$Modal.confirm({
        title: "提示",
        content: "<p>是否删除Flv文件</p>",
        onOk: () => {
          window.ajax.get(
            "//" + location.host + "/api/record/flv/delete",
            { streamPath: item.Path.replace(".flv", "") },
            x => {
              if (x == "success") {
                this.$Message.success("开始发布");
              } else {
                this.$Message.error(x);
              }
            }
          );
        },
        onCancel: () => {}
      });
    },
    toSizeStr(value, unit = "") {
      if (value > 1024 && uintInc[unit]) {
        return this.toSizeStr(value / 1024, uintInc[unit]);
      }
      return value.toFixed(2).replace(".00", "") + unit + "B";
    },
    toDurationStr(value) {
      if (value > 1000) {
        let s = value / 1000;
        if (s > 60) {
          s = s | 0;
          let min = (s / 60) >> 0;
          if (min > 60) {
            let hour = (min / 60) >> 0;
            return hour + "hour" + (min % 60) + "min";
          } else {
            return min + "min" + (s % 60) + "s";
          }
        } else {
          return s.toFixed(3) + "s";
        }
      } else {
        return value + "ms";
      }
    },
    onVisible(visible) {
      if (visible) {
        window.ajax.getJSON(
          "//" + location.host + "/api/record/flv/list",
          {},
          x => {
            this.data = x;
          }
        );
      }
    }
  }
};
</script>

<style scoped>
.records {
  display: flex;
  flex-wrap: wrap;
  padding: 0 15px;
}
.records > * {
  width: 200px;
}
</style>