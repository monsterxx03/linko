# MITM 流量页面 UI 优化设计文档

**日期**: 2026-02-13
**作者**: Claude Code
**状态**: 已批准 ✅

## 概述

本设计文档描述了 `pkg/ui/src/pages/MitmTraffic.tsx` 页面的渐进式优化方案。目标是在保持现有功能的基础上，提升视觉设计和改善代码结构。

## 设计目标

1. **视觉设计改进**: 提升页面美观度和用户体验
2. **代码结构优化**: 改善代码可维护性，减少技术债务
3. **渐进式实施**: 低风险，可逐步实施

## 设计方案

### 1. 视觉设计改进

#### 1.1 颜色系统增强
- **状态颜色优化**:
  - 2xx (成功): 绿色系 (`bg-emerald-*`, `text-emerald-*`)
  - 3xx (重定向): 蓝色系 (`bg-blue-*`, `text-blue-*`)
  - 4xx (客户端错误): 橙色/黄色系 (`bg-amber-*`, `text-amber-*`)
  - 5xx (服务器错误): 红色系 (`bg-red-*`, `text-red-*`)
- **连接状态**: 更明显的在线/离线指示器
- **交互反馈**: 按钮悬停、点击状态的视觉反馈

#### 1.2 卡片设计优化
- **阴影层次**: 增加微妙的阴影提升深度感
- **边框和圆角**: 优化边框颜色和圆角大小
- **内边距**: 调整内边距，改善内容呼吸空间
- **分隔线**: 使用更优雅的分隔方式

#### 1.3 排版和间距
- **字体层次**: 建立清晰的标题-正文-辅助文本层次
- **行高优化**: 增加行高提升可读性
- **组件间距**: 使用一致的间距系统（4px 倍数）
- **对齐方式**: 优化文本对齐，提高视觉一致性

### 2. 组件结构优化（简化版）

#### 2.1 组件拆分结构
```
src/components/mitm/
├── TrafficHeader.tsx          # 顶部状态和控制区域
├── TrafficControls.tsx        # 搜索和过滤控制
├── TrafficItem.tsx           # 单个流量项（包含详情逻辑）
└── shared/
    ├── Badge.tsx             # 通用徽章组件
    ├── CollapsibleSection.tsx # 可折叠区域
    └── CopyButton.tsx        # 复制按钮
```

#### 2.2 组件职责
1. **TrafficHeader** (约 80行)
   - 连接状态显示 + 客户端计数
   - 所有操作按钮（折叠、清除、重连）
   - 状态徽章和统计信息

2. **TrafficControls** (约 40行)
   - 搜索输入框
   - 自动滚动开关
   - 其他过滤控制

3. **TrafficItem** (约 200行)
   - 包含当前 `TrafficItem` 的所有逻辑
   - 集成 `HeadersDisplay`、`JsonBody` 等作为内部函数组件
   - 保持 `MethodBadge`、`StatusBadge` 为内部组件

#### 2.3 共享组件
- **Badge.tsx**: 通用徽章，支持多种颜色变体
- **CollapsibleSection.tsx**: 可折叠面板，用于请求/响应详情
- **CopyButton.tsx**: 复制按钮，带成功状态反馈

### 3. 实现细节

#### 3.1 TypeScript 类型优化
```typescript
// src/types/mitm.ts
export interface TrafficEvent {
  id: string;
  request_id?: string;
  timestamp: number;
  hostname: string;
  direction: 'request' | 'response' | 'complete';
  request?: HttpRequest;
  response?: HttpResponse;
}

export interface HttpRequest {
  method: string;
  url: string;
  headers?: Record<string, string>;
  body?: string;
  content_type?: string;
}

export interface HttpResponse {
  status_code: number;
  status: string;
  headers?: Record<string, string>;
  body?: string;
  content_type?: string;
  latency?: number;
}
```

#### 3.2 样式系统优化
```typescript
// 颜色常量
const COLORS = {
  success: 'bg-emerald-100 text-emerald-800 border-emerald-200',
  warning: 'bg-amber-100 text-amber-800 border-amber-200',
  error: 'bg-red-100 text-red-800 border-red-200',
  info: 'bg-blue-100 text-blue-800 border-blue-200',
} as const;

// 组件类提取
const cardClasses = 'bg-white rounded-xl border border-bg-200 p-4';
const buttonClasses = 'px-4 py-2 text-sm font-medium rounded-lg transition-colors';
```

#### 3.3 性能优化
1. **React.memo**: 对 `TrafficItem` 使用 `React.memo`
2. **useMemo/useCallback**: 优化事件处理和格式化函数
3. **虚拟滚动考虑**: 为未来大量数据预留接口

#### 3.4 错误处理
1. **空状态设计**: 更好的无数据提示
2. **错误边界**: 组件级错误处理
3. **加载状态**: 连接建立期间的加载指示器

## 实施计划

### 阶段 1: 基础重构 (预计 2-3 小时)
1. 创建类型定义文件 (`src/types/mitm.ts`)
2. 提取共享组件 (`Badge`, `CollapsibleSection`, `CopyButton`)
3. 创建组件目录结构

### 阶段 2: 组件拆分 (预计 3-4 小时)
1. 实现 `TrafficHeader` 组件
2. 实现 `TrafficControls` 组件
3. 重构 `TrafficItem` 组件
4. 更新主页面使用新组件

### 阶段 3: 视觉优化 (预计 2-3 小时)
1. 应用新的颜色系统
2. 优化卡片设计和排版
3. 添加交互反馈效果
4. 响应式设计调整

### 阶段 4: 测试和优化 (预计 1-2 小时)
1. 功能测试
2. 性能测试
3. 代码审查和优化

## 成功标准

1. **功能完整性**: 所有现有功能正常工作
2. **视觉提升**: 页面美观度显著提升
3. **代码质量**: 组件结构清晰，类型安全
4. **性能**: 渲染性能不下降，关键操作响应迅速
5. **可维护性**: 代码易于理解和修改

## 风险缓解

1. **功能回归**: 保持现有功能测试，逐步重构
2. **性能影响**: 监控关键性能指标，及时优化
3. **兼容性**: 确保与现有 SSE 事件流兼容
4. **用户体验**: 收集反馈，及时调整设计

## 验收标准

- [ ] 所有现有功能测试通过
- [ ] 页面视觉设计明显改善
- [ ] 组件结构清晰，代码可维护
- [ ] 性能指标符合预期
- [ ] 代码通过 TypeScript 类型检查
- [ ] 响应式设计正常工作

---

*设计已批准，准备进入实施阶段。*