<template>
    <Poptip trigger="hover" :content="'⌚️'+ new Date(value).toLocaleString()">
        <Time :time="new Date(value)"></Time>
    </Poptip>
</template>

<script>
    export default {
        name: "StartTime",
        props:{
            value:String
        }
    }
</script>

<style scoped>

</style>