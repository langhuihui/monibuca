-- 初始化录制计划的 SQL 脚本
-- 包含三个预设计划：工作日全天录制,周末全天录制,每天全天录制

-- 24小时不间断录制计划（每天全天录制）
INSERT INTO record_plans (id, name, plan, enable, created_at, updated_at)
SELECT 1,'每天全天录制',
       -- 168位的计划字符串，格式为：
       -- 前24位为周日，接着24位为周一，以此类推到周六
       -- 0表示不录制，1表示录制
       -- 工作日录制：周一到周五全为1，周六周日全为0
       CONCAT(
           -- 周日（0）：24个1
               REPEAT('1', 24),
           -- 周一（1）：24个1
               REPEAT('1', 24),
           -- 周二（2）：24个1
               REPEAT('1', 24),
           -- 周三（3）：24个1
               REPEAT('1', 24),
           -- 周四（4）：24个1
               REPEAT('1', 24),
           -- 周五（5）：24个1
               REPEAT('1', 24),
           -- 周六（6）：24个1
               REPEAT('1', 24)
       ),
       TRUE, -- 启用状态
       NOW(), -- 创建时间
       NOW()  -- 更新时间
    WHERE NOT EXISTS (
    SELECT 1 FROM record_plans WHERE name = '每天全天录制'
);


-- 工作日计划（周一到周五全天录制）
INSERT INTO record_plans (id,name, plan, enable, created_at, updated_at)
SELECT 2,'工作日录制计划',
       -- 168位的计划字符串，格式为：
       -- 前24位为周日，接着24位为周一，以此类推到周六
       -- 0表示不录制，1表示录制
       -- 工作日录制：周一到周五全为1，周六周日全为0
       CONCAT(
         -- 周日（0）：24个0
         REPEAT('0', 24),
         -- 周一（1）：24个1
         REPEAT('1', 24),
         -- 周二（2）：24个1
         REPEAT('1', 24),
         -- 周三（3）：24个1
         REPEAT('1', 24),
         -- 周四（4）：24个1
         REPEAT('1', 24),
         -- 周五（5）：24个1
         REPEAT('1', 24),
         -- 周六（6）：24个0
         REPEAT('0', 24)
       ),
       TRUE, -- 启用状态
       NOW(), -- 创建时间
       NOW()  -- 更新时间
WHERE NOT EXISTS (
    SELECT 1 FROM record_plans WHERE name = '工作日录制计划'
);

-- 周末计划（周六和周日全天录制）
INSERT INTO record_plans (id,name, plan, enable, created_at, updated_at)
SELECT 3,'周末录制计划',
       -- 168位的计划字符串
       -- 周末录制：周六周日全为1，周一到周五全为0
       CONCAT(
         -- 周日（0）：24个1
         REPEAT('1', 24),
         -- 周一（1）：24个0
         REPEAT('0', 24),
         -- 周二（2）：24个0
         REPEAT('0', 24),
         -- 周三（3）：24个0
         REPEAT('0', 24),
         -- 周四（4）：24个0
         REPEAT('0', 24),
         -- 周五（5）：24个0
         REPEAT('0', 24),
         -- 周六（6）：24个1
         REPEAT('1', 24)
       ),
       TRUE, -- 启用状态
       NOW(), -- 创建时间
       NOW()  -- 更新时间
WHERE NOT EXISTS (
    SELECT 1 FROM record_plans WHERE name = '周末录制计划'
);
