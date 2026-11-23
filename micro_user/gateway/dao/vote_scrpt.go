package dao

import "github.com/redis/go-redis/v9"

var voteScript = redis.NewScript(`
-- 先检查参数。
if #KEYS ~= 2 or #ARGV ~= 3 then
    return redis.error_reply('invalid vote arguments')
end

local newDirection = tonumber(ARGV[3])      
if newDirection ~= 1 and newDirection ~= -1 then
    return redis.error_reply('direction must be 1 or -1')
end

-- 写入前检查两个 key 的类型。
local userVotesType = redis.call('TYPE', KEYS[1]).ok
local voteCountType = redis.call('TYPE', KEYS[2]).ok

if userVotesType ~= 'none' and userVotesType ~= 'zset' then
    return redis.error_reply('invalid user votes key type')
end

if voteCountType ~= 'none' and voteCountType ~= 'zset' then
    return redis.error_reply('invalid vote count key type')
end         

-- 查询旧方向，没有记录时按 0 处理。
local oldDirection = tonumber(
    redis.call('ZSCORE', KEYS[1], ARGV[1]) or '0'
)

if oldDirection ~= 0 and oldDirection ~= 1 and oldDirection ~= -1 then
    return redis.error_reply('invalid stored vote direction')
end

local delta = newDirection - oldDirection

-- 重复同向投票，不修改数据。
if delta == 0 then
    return 0
end

redis.call('ZADD', KEYS[1], newDirection, ARGV[1])
redis.call('ZINCRBY', KEYS[2], delta, ARGV[2])

return delta
`)
