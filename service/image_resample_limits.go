package service

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// 重采样内存与队列边界（对抗评审后的第二版设计）：
//
// 第一版用一个 6 槽共享总闸罩住本机与远端,被证伪:远端超分持闸长达 90s,
// 会把本应亚秒完成的本机缩小和零成本的尺寸一致早退一起堵死/挤掉(优先级
// 反转 + no-op 流量占队)。
//
// 现行设计不设总闸,靠三层各自有界:
//  1. 闸前分类——尺寸一致早退与超上限/非白名单格式/16-bit 的"不适用"只做
//     流式头部解码(读取被 preGateHeadReadLimit 封顶),不碰任何信号量、
//     不做 base64 全量解码。
//  2. 两池隔离——本机缩小(localResampleSem, 4)与远端 RunPod
//     (imageUpscaleSemaphore, 4)各自封顶在飞,远端长任务永远占不到本机的槽;
//     本机过载也不外溢远端(远端是放大的唯一通路,不许被廉价缩小挤占)。
//  3. 有界等待——本机、远端与响应改写三阶段各自限制等待者数量(默认 8/池)。
//     前两池等待者持有 body+b64+已解码源，改写池等待者持有 body+输出图；
//     三个队列均有硬上限，突发流量不会让内存随请求数无界增长。
const defaultResampleMaxWaiters = 8

const (
	maxResampleMaxWaiters             = 32
	defaultImageRewriteMaxConcurrency = 4
)

// ErrResampleOverloaded 表示重采样等待队列已满。本机池满→直接降级(不外溢
// 远端);远端池满→降级。降级=客户拿到上游原图。
var ErrResampleOverloaded = errors.New("image resample queue full")

func resampleLimitEnv(name string, def, max int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// resampleMaxWaiters 读 IMAGE_RESAMPLE_MAX_WAITERS,是"每池"的等待上限
// (本机池与远端池各自适用,计数独立)。
var resampleMaxWaiters = int32(defaultResampleMaxWaiters)

// imageRewriteSem bounds base64 encoding and JSON replacement after local or
// remote resampling. Those allocations are large even though the resampling
// slots have already completed.
var imageRewriteSem = make(chan struct{}, defaultImageRewriteMaxConcurrency)

// InitImageResampleLimits must run after .env loading and before serving
// requests. Package initialization deliberately uses defaults so .env values
// are not frozen before godotenv.Load.
func InitImageResampleLimits() {
	localResampleSem = make(chan struct{}, localResampleMaxConcurrency())
	resampleMaxWaiters = int32(resampleLimitEnv(
		"IMAGE_RESAMPLE_MAX_WAITERS", defaultResampleMaxWaiters, maxResampleMaxWaiters,
	))
	imageRewriteSem = make(chan struct{}, defaultImageRewriteMaxConcurrency)
}
