<template>
  <div>
    <img src="../assets/logo.png">
    <div>
      <p>
        Monibuca 是一个开源的Go语言实现的流媒体服务器开发框架
      </p>
      <Button type="success" to="/docs" target="_blank">🚀START</Button>
      <span style="margin: 0 10px"></span>
      <Button type="default" target="_blank" to="https://github.com/langhuihui/monibuca">
        <svg style="vertical-align: text-top" width="16" height="16" aria-labelledby="simpleicons-github-dark-icon" lang="" role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title id="simpleicons-github-dark-icon" lang="en">GitHub Dark icon</title><path fill="#7F8C8D" d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"></path></svg>
        GITHUB</Button>
    </div>
    <Row style="margin: 30px;">
      <Col span="8">
        <Card :bordered="false" style="margin: 30px">
          <div slot="title" class="feature-title">⚡高性能</div>
          <div>针对流媒体服务器独特的性质进行的优化，充分利用Golang的goroutine的性质对大量的连接的读写进行合理的分配计算资源，以及尽可能的减少内存Copy操作。使用对象池减少Golang的GC时间。</div>
        </Card>
      </Col>
      <Col span="8">
        <Card :bordered="false" style="margin: 30px">
          <div slot="title" class="feature-title">🔧可扩展</div>
          <div>流媒体服务器的个性化定制变的更简单，基于Golang语言，开发效率更高，独创的插件机制，可以方便用户定制个性化的功能组合，更高效率的利用服务器资源。</div>
        </Card>
      </Col>
      <Col span="8">
        <Card :bordered="false" style="margin: 30px">
          <div slot="title" class="feature-title">📈可视化</div>
          <div>功能强大的仪表盘可以直观的看到服务器运行的状态、消耗的资源、以及其他统计信息。用户可以利用控制台对服务器进行配置和控制。点击右上角菜单栏里面的控制台，可以看到演示。</div>
        </Card>
      </Col>
    </Row>
  </div>
</template>

<script>

</script>
<style scoped>
  .feature-title{
    color: #eb5e46;
    font-weight: bold;
    font-size: larger;
  }
  p {
    margin: 30px;
    font-size: 20px;
  }
  img{
    margin: 20px;
  }
</style>
