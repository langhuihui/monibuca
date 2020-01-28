<template>
    <Modal
            v-bind="$attrs" draggable
            v-on="$listeners"
            :title="url"
            @on-ok="onClosePreview"
            @on-cancel="onClosePreview">
        <canvas id="canvas" width="488" height="275" style="background: black"/>
    </Modal>
</template>

<script>
    let h5lc = null;
    export default {
        name: 'Jessibuca',
        props: {
            audioEnabled: Boolean,
        },
        data(){
            return {
                url:""
            }
        },
        watch: {
            audioEnabled(value){
              h5lc.audioEnabled(value)
            }
        },
        mounted() {
            h5lc = new window.Jessibuca({
                canvas: document.getElementById("canvas"),
                decoder: "jessibuca/ff.js"
            });
        },
        destroyed() {
            this.onClosePreview()
            h5lc.destroy()
        },
        methods: {
            play(url){
                this.url = url
                h5lc.play(url)
            },
            onClosePreview() {
                h5lc.close();
            },
        }
    }
</script>

