<template>
  <div class="page">
    <div class="coming-soon">
      <h2>安全设置</h2>
      <p class="coming-text">开发中...</p>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use '@/styles/variables.scss' as *;

.page {
  padding: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  height: calc(100vh - 56px);
}

.coming-soon {
  text-align: center;
}

h2 {
  color: $text-primary;
  font-weight: 500;
  margin-bottom: 8px;
}

.coming-text {
  color: $text-secondary;
}
</style>