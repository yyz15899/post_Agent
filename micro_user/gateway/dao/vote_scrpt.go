package dao

import "github.com/redis/go-redis/v9"

const voteScoreCacheMiss = "vote score cache need rebuilding"

var voteScript = redis.NewScript(`
-- 先检查参数。
if #KEYS ~= 3 or #ARGV ~= 4 then
    return redis.error_reply('invalid vote arguments')
end

local newDirection = tonumber(ARGV[3])      
if newDirection ~= 1 and newDirection ~= -1 then
    return redis.error_reply('direction must be 1 or -1')
end

-- 写入前检查三个 key 的类型。
local userVotesType = redis.call('TYPE', KEYS[1]).ok
local voteCountType = redis.call('TYPE', KEYS[2]).ok
local voteversionType = redis.call('TYPE',KEYS[3]).ok

-- 校验类型
if userVotesType ~= 'none' and userVotesType ~= 'zset' then
    return redis.error_reply('invalid user votes key type')
end

if voteCountType ~= 'none' and voteCountType ~= 'zset' then
    return redis.error_reply('invalid vote count key type')
end         

if voteversionType ~= 'none' and voteversionType ~= 'hash' then
    return redis.error_reply('invalid vote version key type')
end

--  拿到此时post对应总分数  判断此时redis中是否有值(redis中的值消失时 会先从DB中取出vote_num 这里要防止重复计分)
local currentVoteScore = redis.call('ZSCORE',KEYS[2],ARGV[2])
if not currentVoteScore then
    return redis.error_reply('` + voteScoreCacheMiss + `')
end

-- 判断是否有旧版本但没有旧方向(direction数据消失)
local rawOldVersion = redis.call('HGET', KEYS[3], ARGV[1])
local rawOldDirection = redis.call('ZSCORE', KEYS[1], ARGV[1])

if rawOldVersion and not rawOldDirection then
    redis.call('ZREM', KEYS[2], ARGV[2])
    return redis.error_reply('` + voteScoreCacheMiss + `')
end

local oldVersion = rawOldVersion or '0'
-- 这里保留原来的版本比较
local oldDirection = tonumber(rawOldDirection or '0')

local newVersion = ARGV[4]

if #newVersion < #oldVersion or
    (#newVersion == #oldVersion and newVersion <= oldVersion) then
    return 0
end


if oldDirection ~= 0 and oldDirection ~= 1 and oldDirection ~= -1 then
    return redis.error_reply('invalid stored vote direction')
end


local delta = newDirection - oldDirection

-- 踩变赞(赞变踩)
if delta ~= 0 then
    redis.call('ZADD', KEYS[1], newDirection, ARGV[1])   -- 用户看到自己的投票方向
    redis.call('ZINCRBY', KEYS[2], delta, ARGV[2])      -- 帖子总票数的增量
end

redis.call('HSET',KEYS[3],ARGV[1],newVersion)
return delta
`)

// 确保协调多个请求同时恢复同一redis的任务
var unlockPostVoteRestoreScript = redis.NewScript(`
if redis.call('GET',KEYS[1]) == ARGV[1] then
    return redis.call('DEL',KEYS[1])
end
return 0
`)

// 原子恢复redis操作
var restorePostVoteSnapshotScript = redis.NewScript(`
if #KEYS ~= 4 or #ARGV < 2 or (#ARGV - 2) % 3 ~= 0 then
    return redis.error_reply('invalid restore arguments')
end

-- 锁已过期或属于其他恢复者时，拒绝写回。
if redis.call('GET', KEYS[4]) ~= ARGV[1] then
    return 0
end

-- 总分已经存在时，保留当前缓存。
if redis.call('ZSCORE', KEYS[2], ARGV[2]) then
    return 2
end

-- 先检查全部数据，并计算总分。
local score = 0
for i = 3, #ARGV, 3 do
    local direction = tonumber(ARGV[i + 1])
    if direction ~= -1 and direction ~= 0 and direction ~= 1 then
        return redis.error_reply('invalid snapshot direction')
    end
    if not string.match(ARGV[i + 2], '^[1-9]%d*$') then
        return redis.error_reply('invalid snapshot version')
    end
    score = score + direction
end

-- 重建当前帖子的方向和版本。
redis.call('DEL', KEYS[1], KEYS[3])
for i = 3, #ARGV, 3 do
    redis.call('ZADD', KEYS[1], ARGV[i + 1], ARGV[i])
    redis.call('HSET', KEYS[3], ARGV[i], ARGV[i + 2])
end

redis.call('ZADD', KEYS[2], score, ARGV[2])
return 1
`)
