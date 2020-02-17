<template>
    <div>
        <i-input ref="input" v-bind="$attrs" v-on="$listeners" clearable @on-change="onInput">
            <Button slot="prepend" icon="md-arrow-round-up" @click="goUp"></Button>
        </i-input>
        <CellGroup @on-click="onSelectCand">
            <Cell v-for="item in candidate" :key="item" :title="item" :name="item"></Cell>
        </CellGroup>
    </div>
</template>

<script>
    export default {
        name: "PathSelector",
        data() {
            return {
                candidate: [],
                lastInput: "",
                searching: false,
            }
        },
        methods: {
            dir(){
                let paths = this.$refs.input.value.split("/");
                paths.pop();
                return paths.join("/");
            },
            goUp() {
                this.lastInput = this.$attrs.value = this.dir()
                this.$refs.input.$emit('input', this.$attrs.value)
                this.search(this.lastInput)
            },
            onSelectCand(name) {
                this.lastInput = this.$attrs.value = this.dir()+"/"+name+"/"
                this.$refs.input.$emit('input', this.$attrs.value)
                this.search(this.lastInput)
            },
            onInput(evt) {
                this.lastInput = evt.target.value
                this.search(this.lastInput)
            },
            search(v) {
                if(this.searching)return
                window.ajax.getJSON("/instance/listDir?input=" + v).then(x => {
                    this.candidate = x
                    if (this.lastInput != v) {
                        this.search(this.lastInput)
                    }else{
                        this.searching = false
                    }
                }).catch(e => {
                    this.$Message.error(e)
                    if (this.lastInput != v) {
                        this.search(this.lastInput)
                    }else{
                        this.searching = false
                    }
                })
            }
        }
    }
</script>

<style scoped>

</style>