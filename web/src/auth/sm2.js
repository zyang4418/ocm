/**
 * SM2 国密加密工具
 * 用于前端密码应用层传输加密，基于 C1C3C2 模式与 Base64 编码。
 */
import { sm2 } from 'sm-crypto'

// 缓存公钥和指纹
let cachedPublicKey = null
let cachedFingerprint = null

const canonicalPublicKeyPattern = /^[0-9a-f]{128}$/i
const prefixedPublicKeyPattern = /^04[0-9a-f]{128}$/i
const hexBytesPattern = /^(?:[0-9a-f]{2})+$/i

function hexToBytes(hex, errorMessage) {
  if (typeof hex !== 'string' || !hexBytesPattern.test(hex)) {
    throw new Error(errorMessage)
  }
  const bytes = new Uint8Array(hex.length / 2)
  for (let index = 0; index < bytes.length; index++) {
    bytes[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16)
  }
  return bytes
}

/**
 * 计算 64 字节规范公钥 Hex (X||Y) 的 SHA-256 指纹
 * @param {string} canonicalPubHex - 128 字符 Hex 字符串
 * @returns {Promise<{fp16: string, fpFull: string}>}
 */
export async function computeFingerprint(canonicalPubHex) {
  if (typeof canonicalPubHex !== 'string' || !canonicalPublicKeyPattern.test(canonicalPubHex)) {
    throw new Error('无效的公钥Hex（必须为64字节）')
  }
  const bytes = hexToBytes(canonicalPubHex, '无效的公钥Hex（必须为64字节）')
  const hashBuffer = await crypto.subtle.digest('SHA-256', bytes)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  const fpFull = hashHex.toUpperCase()
  const fp16 = fpFull.substring(0, 16)
  return { fp16, fpFull }
}

/**
 * 获取 SM2 公钥并校验指纹与 Pinning 策略
 * @returns {Promise<{public_key: string, fingerprint: string}>}
 */
export async function fetchPublicKey() {
  if (cachedPublicKey && cachedFingerprint) {
    return { public_key: cachedPublicKey, fingerprint: cachedFingerprint }
  }

  let response
  try {
    response = await fetch('/api/auth/sm2/public-key')
  } catch {
    clearPublicKeyCache()
    throw new Error('无法连接到服务器获取公钥')
  }

  if (!response.ok) {
    clearPublicKeyCache()
    throw new Error(`获取公钥失败（${response.status}）`)
  }

  const data = await response.json().catch(() => null)
  const result = data?.data || data
  if (!result?.public_key || !result?.fingerprint) {
    clearPublicKeyCache()
    throw new Error('公钥响应格式不正确')
  }

  const rawPub = String(result.public_key).trim()
  let canonicalPub
  if (canonicalPublicKeyPattern.test(rawPub)) {
    // A 64-byte X||Y key is already canonical even if X begins with 04.
    canonicalPub = rawPub.toLowerCase()
  } else if (prefixedPublicKeyPattern.test(rawPub)) {
    canonicalPub = rawPub.slice(2).toLowerCase()
  } else {
    clearPublicKeyCache()
    throw new Error('公钥格式异常（必须为64字节坐标或带04前缀的65字节公钥）')
  }

  const { fp16, fpFull } = await computeFingerprint(canonicalPub)
  const serverFingerprint = String(result.fingerprint).trim().toUpperCase()
  // New servers return the full digest; accept the historical 16-character
  // prefix so a rolling deployment or an older backend remains interoperable.
  if (serverFingerprint !== fp16 && serverFingerprint !== fpFull) {
    clearPublicKeyCache()
    throw new Error('公钥指纹验证失败，可能存在传输被篡改')
  }

  // 生产环境强制要求配置且匹配指纹 Pin
  const pinnedFp = (import.meta.env.VITE_SM2_PUBLIC_KEY_FINGERPRINT || '').trim().toUpperCase()
  const isProd = import.meta.env.PROD || import.meta.env.MODE === 'production'

  if (isProd) {
    if (!pinnedFp) {
      clearPublicKeyCache()
      throw new Error('安全策略要求：未配置公钥指纹 (VITE_SM2_PUBLIC_KEY_FINGERPRINT)')
    }
    if (pinnedFp !== fp16 && pinnedFp !== fpFull) {
      clearPublicKeyCache()
      throw new Error('安全策略要求：公钥指纹校验失败，可能存在中间人劫持')
    }
  } else if (pinnedFp) {
    if (pinnedFp !== fp16 && pinnedFp !== fpFull) {
      clearPublicKeyCache()
      throw new Error('公钥指纹与环境变量配置不匹配')
    }
  }

  cachedPublicKey = canonicalPub
  cachedFingerprint = fpFull

  return { public_key: canonicalPub, fingerprint: fpFull }
}

/**
 * SM2 加密密码
 * @param {string} plaintext - 待加密明文
 * @returns {Promise<string>} - Base64 编码的密文
 */
export async function encryptPassword(plaintext) {
  if (!plaintext) {
    throw new Error('密码不能为空')
  }

  const { public_key } = await fetchPublicKey()
  // sm-crypto 需要 '04' 前缀表示非压缩公钥格式
  const fullPublicKey = '04' + public_key

  // 注意：sm-crypto 的第三个参数 1 表示 C1C3C2 模式（对应 Go gmsm 的 sm2.C1C3C2）
  const cipherHex = sm2.doEncrypt(plaintext, fullPublicKey, 1)
  if (!cipherHex) {
    clearPublicKeyCache()
    throw new Error('SM2 加密运算失败')
  }

  let bytes
  try {
    bytes = hexToBytes(cipherHex, '加密密文Hex解析失败')
  } catch (error) {
    clearPublicKeyCache()
    throw error
  }
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

/**
 * 清除缓存的公钥与指纹
 */
export function clearPublicKeyCache() {
  cachedPublicKey = null
  cachedFingerprint = null
}
