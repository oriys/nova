"use client"

import { useRouter } from "next/navigation"
import { useEffect, useState, useRef, useCallback } from "react"

const baseFunctions = [
  { lang: "python", icon: "devicon-python-plain", name: "process_image", code: "def handler(e):\n  return resize(e)" },
  { lang: "javascript", icon: "devicon-javascript-plain", name: "send_email", code: "export const handler = (e) =>\n  sendMail(e)" },
  { lang: "go", icon: "devicon-go-plain", name: "compress_data", code: "func Handler(e Event) {\n  return Compress(e)\n}" },
  { lang: "rust", icon: "devicon-rust-original", name: "validate_token", code: "pub fn handler(e: Event) {\n  validate(&e)\n}" },
  { lang: "java", icon: "devicon-java-plain", name: "generateReport", code: "Response handler(Event e) {\n  return gen(e);\n}" },
  { lang: "typescript", icon: "devicon-typescript-plain", name: "parse_webhook", code: "export const handler = (e: E) =>\n  parse(e)" },
  { lang: "ruby", icon: "devicon-ruby-plain", name: "sync_database", code: "def handler(event)\n  sync(event)\nend" },
  { lang: "php", icon: "devicon-php-plain", name: "cache_response", code: "function handler($e) {\n  return cache($e);\n}" },
  { lang: "csharp", icon: "devicon-csharp-plain", name: "ProcessOrder", code: "Response Handler(Event e) {\n  return Do(e);\n}" },
  { lang: "python", icon: "devicon-python-plain", name: "analyze_text", code: "def handler(e):\n  return nlp(e)" },
  { lang: "javascript", icon: "devicon-nodejs-plain", name: "fetch_data", code: "export const handler = (e) =>\n  fetch(e.url)" },
  { lang: "go", icon: "devicon-go-plain", name: "handle_request", code: "func Handler(w, r) {\n  json.Encode(w, r)\n}" },
  { lang: "rust", icon: "devicon-rust-original", name: "encrypt_payload", code: "pub fn handler(e: Event) {\n  encrypt(&e)\n}" },
  { lang: "java", icon: "devicon-java-plain", name: "convertFormat", code: "Response handler(Event e) {\n  return conv(e);\n}" },
  { lang: "typescript", icon: "devicon-typescript-plain", name: "validate_input", code: "export const handler = (e: E) =>\n  check(e)" },
  { lang: "python", icon: "devicon-python-plain", name: "train_model", code: "def handler(e):\n  return train(e)" },
  { lang: "go", icon: "devicon-go-plain", name: "stream_logs", code: "func Handler(e Event) {\n  return Stream(e)\n}" },
  { lang: "ruby", icon: "devicon-ruby-plain", name: "notify_user", code: "def handler(event)\n  notify(event)\nend" },
  { lang: "javascript", icon: "devicon-javascript-plain", name: "transform_data", code: "export const handler = (e) =>\n  transform(e)" },
  { lang: "rust", icon: "devicon-rust-original", name: "parse_config", code: "pub fn handler(e: Event) {\n  parse(&e)\n}" },
]

const startLanguages = [
  { lang: "python", icon: "devicon-python-plain" },
  { lang: "javascript", icon: "devicon-javascript-plain" },
  { lang: "go", icon: "devicon-go-plain" },
  { lang: "rust", icon: "devicon-rust-original" },
  { lang: "typescript", icon: "devicon-typescript-plain" },
]

interface FunctionCard {
  id: string
  lang: string
  icon: string
  name: string
  code: string
  isStart: boolean
  x: number
  y: number
  scale: number
  rotation: number
  flipped: boolean
  velocity: { x: number; y: number }
  rainbow: boolean
  exploding: boolean
  frozen: boolean
}

interface MatrixDrop {
  x: number
  y: number
  speed: number
  chars: string[]
}

export default function LandingPage() {
  const router = useRouter()
  const [mousePos, setMousePos] = useState({ x: -1000, y: -1000 })
  const [mounted, setMounted] = useState(false)
  const [cards, setCards] = useState<FunctionCard[]>([])
  const [dragging, setDragging] = useState<string | null>(null)
  const [konamiProgress, setKonamiProgress] = useState(0)
  const [partyMode, setPartyMode] = useState(false)
  const [clickCounts, setClickCounts] = useState<Record<string, number>>({})
  const [gravityMode, setGravityMode] = useState(false)
  const [matrixMode, setMatrixMode] = useState(false)
  const [matrixDrops, setMatrixDrops] = useState<MatrixDrop[]>([])
  const [blackHole, setBlackHole] = useState<{ x: number; y: number } | null>(null)
  const blackHoleStartRef = useRef<number>(0)
  const [chargeLevel, setChargeLevel] = useState(0)
  const [typedKeys, setTypedKeys] = useState("")
  const [slowMo, setSlowMo] = useState(false)
  const [tornado, setTornado] = useState(false)
  const [mouseHistory, setMouseHistory] = useState<{ x: number; y: number; t: number }[]>([])
  const [fruitNinjaMode, setFruitNinjaMode] = useState(false)
  const [slashTrail, setSlashTrail] = useState<{ x: number; y: number; t: number }[]>([])
  const swipeCountRef = useRef(0)
  const lastSwipeDirectionRef = useRef<'left' | 'right' | null>(null)
  const lastMouseXRef = useRef(0)
  const dragOffset = useRef({ x: 0, y: 0 })
  const lastDragPos = useRef({ x: 0, y: 0 })

  const konamiCode = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'KeyB', 'KeyA']

  // Initialize cards
  useEffect(() => {
    setMounted(true)
    const startLang = startLanguages[Math.floor(Math.random() * startLanguages.length)]
    const initialCards: FunctionCard[] = [
      ...baseFunctions.map((fn, i) => ({
        id: `fn-${i}`,
        ...fn,
        isStart: false,
        x: Math.random() * 80 + 10,
        y: Math.random() * 75 + 10,
        scale: Math.random() * 0.3 + 0.7,
        rotation: 0,
        flipped: false,
        velocity: { x: 0, y: 0 },
        rainbow: false,
        exploding: false,
        frozen: false,
      })),
      {
        id: 'start',
        lang: startLang.lang,
        icon: startLang.icon,
        name: 'start',
        code: '// Click to enter\nrouter.push("/dashboard")',
        isStart: true,
        x: Math.random() * 80 + 10,
        y: Math.random() * 75 + 10,
        scale: Math.random() * 0.3 + 0.7,
        rotation: 0,
        flipped: false,
        velocity: { x: 0, y: 0 },
        rainbow: false,
        exploding: false,
        frozen: false,
      },
    ]
    setCards(initialCards)
  }, [])

  // Keyboard listener for Konami + text commands
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Konami code
      if (e.code === konamiCode[konamiProgress]) {
        const newProgress = konamiProgress + 1
        setKonamiProgress(newProgress)
        if (newProgress === konamiCode.length) {
          setPartyMode(true)
          setCards(prev => prev.map(card => ({ ...card, rainbow: true })))
          setTimeout(() => {
            setPartyMode(false)
            setCards(prev => prev.map(card => ({ ...card, rainbow: false })))
          }, 5000)
          setKonamiProgress(0)
        }
      } else if (!e.code.startsWith('Key')) {
        setKonamiProgress(0)
      }

      // Text commands
      if (e.code.startsWith('Key')) {
        const key = e.code.replace('Key', '').toLowerCase()
        const newTyped = (typedKeys + key).slice(-10)
        setTypedKeys(newTyped)

        // "gravity" - 重力模式
        if (newTyped.endsWith('gravity')) {
          setGravityMode(true)
          setTimeout(() => setGravityMode(false), 5000)
          setTypedKeys("")
        }
        // "matrix" - 黑客帝国
        if (newTyped.endsWith('matrix')) {
          setMatrixMode(true)
          const drops: MatrixDrop[] = Array.from({ length: 30 }, (_, i) => ({
            x: (i / 30) * 100,
            y: Math.random() * -100,
            speed: Math.random() * 2 + 1,
            chars: Array.from({ length: 20 }, () => String.fromCharCode(0x30A0 + Math.random() * 96)),
          }))
          setMatrixDrops(drops)
          setTimeout(() => {
            setMatrixMode(false)
            setMatrixDrops([])
          }, 8000)
          setTypedKeys("")
        }
        // "freeze" - 冻结所有卡片
        if (newTyped.endsWith('freeze')) {
          setCards(prev => prev.map(card => ({ ...card, frozen: true, velocity: { x: 0, y: 0 } })))
          setTimeout(() => {
            setCards(prev => prev.map(card => ({ ...card, frozen: false })))
          }, 3000)
          setTypedKeys("")
        }
        // "boom" - 所有卡片爆炸散开
        if (newTyped.endsWith('boom')) {
          setCards(prev => prev.map(card => ({
            ...card,
            velocity: {
              x: (Math.random() - 0.5) * 20,
              y: (Math.random() - 0.5) * 20,
            }
          })))
          setTypedKeys("")
        }
        // "sort" - 卡片排序整齐
        if (newTyped.endsWith('sort')) {
          setCards(prev => {
            const sorted = [...prev]
            const cols = 5
            return sorted.map((card, i) => ({
              ...card,
              x: 15 + (i % cols) * 17,
              y: 15 + Math.floor(i / cols) * 18,
              rotation: 0,
              velocity: { x: 0, y: 0 },
            }))
          })
          setTypedKeys("")
        }
      }

      // Space for slow motion
      if (e.code === 'Space') {
        e.preventDefault()
        setSlowMo(true)
      }
    }

    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.code === 'Space') {
        setSlowMo(false)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    window.addEventListener('keyup', handleKeyUp)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('keyup', handleKeyUp)
    }
  }, [konamiProgress, typedKeys])

  // Physics + gravity + black hole + tornado
  useEffect(() => {
    if (!mounted) return
    const speed = slowMo ? 0.2 : 1
    const interval = setInterval(() => {
      setCards(prev => prev.map(card => {
        if (card.frozen) return card
        if (Math.abs(card.velocity.x) < 0.01 && Math.abs(card.velocity.y) < 0.01 && !gravityMode && !blackHole && !tornado) return card

        let newVelX = card.velocity.x * (slowMo ? 0.99 : 0.95)
        let newVelY = card.velocity.y * (slowMo ? 0.99 : 0.95)

        // Gravity
        if (gravityMode) {
          newVelY += 0.5 * speed
        }

        // Black hole attraction
        if (blackHole) {
          const dx = blackHole.x - card.x
          const dy = blackHole.y - card.y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist > 5) {
            const force = 50 / (dist * dist) * speed
            newVelX += (dx / dist) * force
            newVelY += (dy / dist) * force
          }
        }

        // Tornado
        if (tornado) {
          const cx = 50, cy = 50
          const dx = card.x - cx
          const dy = card.y - cy
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist > 0) {
            // Spiral force
            newVelX += (-dy / dist) * 0.5 * speed - dx * 0.02
            newVelY += (dx / dist) * 0.5 * speed - dy * 0.02
          }
        }

        let newX = card.x + newVelX * speed
        let newY = card.y + newVelY * speed

        // Bounce
        if (newX < 5 || newX > 95) newVelX = -newVelX * 0.8
        if (newY < 5 || newY > 95) newVelY = -newVelY * 0.8
        newX = Math.max(5, Math.min(95, newX))
        newY = Math.max(5, Math.min(95, newY))

        return { ...card, x: newX, y: newY, velocity: { x: newVelX, y: newVelY } }
      }))

      // Matrix drops animation
      if (matrixMode) {
        setMatrixDrops(prev => prev.map(drop => ({
          ...drop,
          y: drop.y > 110 ? -20 : drop.y + drop.speed,
        })))
      }
    }, 16)
    return () => clearInterval(interval)
  }, [mounted, gravityMode, blackHole, slowMo, tornado, matrixMode])

  // Party mode animation
  useEffect(() => {
    if (!partyMode) return
    const interval = setInterval(() => {
      setCards(prev => prev.map(card => ({
        ...card,
        rotation: card.rotation + (Math.random() - 0.5) * 10,
      })))
    }, 100)
    return () => clearInterval(interval)
  }, [partyMode])

  // Tornado detection - circular mouse movement
  useEffect(() => {
    if (mouseHistory.length < 20) return
    const recent = mouseHistory.slice(-20)
    let totalAngle = 0
    for (let i = 1; i < recent.length; i++) {
      const dx1 = recent[i - 1].x - window.innerWidth / 2
      const dy1 = recent[i - 1].y - window.innerHeight / 2
      const dx2 = recent[i].x - window.innerWidth / 2
      const dy2 = recent[i].y - window.innerHeight / 2
      const angle1 = Math.atan2(dy1, dx1)
      const angle2 = Math.atan2(dy2, dx2)
      let diff = angle2 - angle1
      if (diff > Math.PI) diff -= 2 * Math.PI
      if (diff < -Math.PI) diff += 2 * Math.PI
      totalAngle += diff
    }
    if (Math.abs(totalAngle) > Math.PI * 3 && !tornado) {
      setTornado(true)
      setTimeout(() => setTornado(false), 3000)
    }
  }, [mouseHistory, tornado])

  const handleMouseMove = useCallback((e: MouseEvent) => {
    setMousePos({ x: e.clientX, y: e.clientY })

    // Track mouse history for tornado detection
    setMouseHistory(prev => [...prev.slice(-30), { x: e.clientX, y: e.clientY, t: Date.now() }])

    // Detect left-right swipes for fruit ninja mode activation
    const deltaX = e.clientX - lastMouseXRef.current
    if (Math.abs(deltaX) > 50) {
      const direction = deltaX > 0 ? 'right' : 'left'
      if (lastSwipeDirectionRef.current && lastSwipeDirectionRef.current !== direction) {
        swipeCountRef.current++
        if (swipeCountRef.current >= 10 && !fruitNinjaMode) {
          setFruitNinjaMode(true)
          swipeCountRef.current = 0
          setTimeout(() => setFruitNinjaMode(false), 8000)
        }
      }
      lastSwipeDirectionRef.current = direction
      lastMouseXRef.current = e.clientX
    }

    // Fruit ninja mode - slash through cards
    if (fruitNinjaMode) {
      setSlashTrail(prev => [...prev.slice(-20), { x: e.clientX, y: e.clientY, t: Date.now() }])

      // Check if mouse path intersects with any card
      const mouseXPercent = (e.clientX / window.innerWidth) * 100
      const mouseYPercent = (e.clientY / window.innerHeight) * 100

      setCards(prev => {
        const newCards: FunctionCard[] = []
        let sliced = false

        prev.forEach(card => {
          const dx = mouseXPercent - card.x
          const dy = mouseYPercent - card.y
          const dist = Math.sqrt(dx * dx + dy * dy)

          if (dist < 8 && !card.isStart && !card.exploding) {
            // Slice the card into two halves
            sliced = true
            const sliceAngle = Math.atan2(dy, dx)
            newCards.push({
              ...card,
              id: `${card.id}-slice-l-${Date.now()}`,
              velocity: {
                x: -Math.cos(sliceAngle + Math.PI/2) * 8,
                y: -Math.sin(sliceAngle + Math.PI/2) * 8 - 3
              },
              rotation: card.rotation - 30,
              scale: card.scale * 0.8,
            })
            newCards.push({
              ...card,
              id: `${card.id}-slice-r-${Date.now()}`,
              velocity: {
                x: Math.cos(sliceAngle + Math.PI/2) * 8,
                y: Math.sin(sliceAngle + Math.PI/2) * 8 - 3
              },
              rotation: card.rotation + 30,
              scale: card.scale * 0.8,
            })
          } else {
            newCards.push(card)
          }
        })

        return sliced ? newCards : prev
      })
    }

    if (dragging !== null) {
      const newX = ((e.clientX - dragOffset.current.x) / window.innerWidth) * 100
      const newY = ((e.clientY - dragOffset.current.y) / window.innerHeight) * 100

      setCards(prev => prev.map(card => {
        if (card.id === dragging) {
          return { ...card, x: newX, y: newY }
        }
        const dx = newX - card.x
        const dy = newY - card.y
        const dist = Math.sqrt(dx * dx + dy * dy)
        if (dist < 15 && dist > 0) {
          const pushForce = (15 - dist) / 15 * 3
          return {
            ...card,
            velocity: {
              x: card.velocity.x - (dx / dist) * pushForce,
              y: card.velocity.y - (dy / dist) * pushForce,
            }
          }
        }
        return card
      }))
      lastDragPos.current = { x: e.clientX, y: e.clientY }
    }
  }, [dragging, fruitNinjaMode])

  const handleMouseUp = useCallback((e: MouseEvent) => {
    if (dragging !== null) {
      const velX = (e.clientX - lastDragPos.current.x) * 0.1
      const velY = (e.clientY - lastDragPos.current.y) * 0.1
      if (Math.abs(velX) > 1 || Math.abs(velY) > 1) {
        setCards(prev => prev.map(card =>
          card.id === dragging ? { ...card, velocity: { x: velX, y: velY } } : card
        ))
      }
    }
    setDragging(null)
    setBlackHole(null)
  }, [dragging])

  useEffect(() => {
    window.addEventListener("mousemove", handleMouseMove)
    window.addEventListener("mouseup", handleMouseUp)
    return () => {
      window.removeEventListener("mousemove", handleMouseMove)
      window.removeEventListener("mouseup", handleMouseUp)
    }
  }, [handleMouseMove, handleMouseUp])

  const handleMouseDown = (e: React.MouseEvent, cardId: string) => {
    e.preventDefault()
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    dragOffset.current = { x: e.clientX - rect.left - rect.width / 2, y: e.clientY - rect.top - rect.height / 2 }
    lastDragPos.current = { x: e.clientX, y: e.clientY }
    setDragging(cardId)
    setCards(prev => prev.map(card => card.id === cardId ? { ...card, velocity: { x: 0, y: 0 } } : card))
  }

  // Charge level animation
  useEffect(() => {
    if (!blackHole) {
      setChargeLevel(0)
      return
    }
    const interval = setInterval(() => {
      setChargeLevel(Math.min((Date.now() - blackHoleStartRef.current) / 30, 100))
    }, 16)
    return () => clearInterval(interval)
  }, [blackHole])

  // Right click and hold to create black hole
  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault()
  }

  const handleMouseDownGlobal = useCallback((e: MouseEvent) => {
    if (e.button === 2) { // Right click
      const x = (e.clientX / window.innerWidth) * 100
      const y = (e.clientY / window.innerHeight) * 100
      setBlackHole({ x, y })
      blackHoleStartRef.current = Date.now()
    }
  }, [])

  useEffect(() => {
    const handleMouseMoveForBlackHole = (e: MouseEvent) => {
      if (blackHole) {
        const x = (e.clientX / window.innerWidth) * 100
        const y = (e.clientY / window.innerHeight) * 100
        setBlackHole({ x, y })
      }
    }

    const handleMouseUpForBlackHole = (e: MouseEvent) => {
      if (e.button === 2 && blackHole) {
        // Calculate hold duration and explode cards outward
        const holdDuration = Date.now() - blackHoleStartRef.current
        const power = Math.min(holdDuration / 100, 30) // Max power after 3 seconds

        setCards(prev => prev.map(card => {
          const dx = card.x - blackHole.x
          const dy = card.y - blackHole.y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist > 0) {
            const force = power / Math.max(dist / 20, 1)
            return {
              ...card,
              velocity: {
                x: (dx / dist) * force,
                y: (dy / dist) * force,
              }
            }
          }
          return card
        }))

        setBlackHole(null)
        blackHoleStartRef.current = 0
      }
    }

    window.addEventListener('mousedown', handleMouseDownGlobal)
    window.addEventListener('mousemove', handleMouseMoveForBlackHole)
    window.addEventListener('mouseup', handleMouseUpForBlackHole)
    return () => {
      window.removeEventListener('mousedown', handleMouseDownGlobal)
      window.removeEventListener('mousemove', handleMouseMoveForBlackHole)
      window.removeEventListener('mouseup', handleMouseUpForBlackHole)
    }
  }, [blackHole, handleMouseDownGlobal])

  const handleDoubleClick = (cardId: string) => {
    setCards(prev => prev.map(card => card.id === cardId ? { ...card, flipped: !card.flipped } : card))
  }

  // Triple click to explode card into particles
  const handleTripleClick = (cardId: string) => {
    setCards(prev => prev.map(card => {
      if (card.id === cardId) {
        return { ...card, exploding: true }
      }
      return card
    }))
    setTimeout(() => {
      setCards(prev => prev.filter(card => card.id !== cardId))
    }, 500)
  }

  const handleClick = (card: FunctionCard, e: React.MouseEvent) => {
    if (e.detail === 3) {
      handleTripleClick(card.id)
      return
    }
    if (card.isStart) {
      router.push("/dashboard")
      return
    }
    const newCount = (clickCounts[card.id] || 0) + 1
    setClickCounts(prev => ({ ...prev, [card.id]: newCount }))
    setTimeout(() => setClickCounts(prev => ({ ...prev, [card.id]: 0 })), 500)
    if (newCount >= 5) {
      const newCard: FunctionCard = {
        ...card,
        id: `${card.id}-clone-${Date.now()}`,
        x: card.x + (Math.random() - 0.5) * 10,
        y: card.y + (Math.random() - 0.5) * 10,
        velocity: { x: (Math.random() - 0.5) * 5, y: (Math.random() - 0.5) * 5 },
      }
      setCards(prev => [...prev, newCard])
      setClickCounts(prev => ({ ...prev, [card.id]: 0 }))
    }
  }

  // Double click background to shuffle
  const handleBackgroundDoubleClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      setCards(prev => prev.map(card => ({
        ...card,
        x: Math.random() * 80 + 10,
        y: Math.random() * 75 + 10,
        velocity: { x: (Math.random() - 0.5) * 3, y: (Math.random() - 0.5) * 3 },
      })))
    }
  }

  if (!mounted || cards.length === 0) {
    return <div className="h-screen w-screen bg-black" />
  }

  // Calculate opacity based on distance from mouse (0-1)
  const getCardOpacity = (card: FunctionCard) => {
    const cardPixelX = (card.x / 100) * window.innerWidth
    const cardPixelY = (card.y / 100) * window.innerHeight
    const dx = cardPixelX - mousePos.x
    const dy = cardPixelY - mousePos.y
    const dist = Math.sqrt(dx * dx + dy * dy)
    const maxDist = 300 // Cards beyond this distance are invisible
    const opacity = Math.max(0, 1 - dist / maxDist)
    return opacity
  }

  return (
    <div
      className={`relative h-screen w-screen overflow-hidden bg-black cursor-none select-none ${partyMode ? 'animate-pulse' : ''}`}
      onContextMenu={handleContextMenu}
      onDoubleClick={handleBackgroundDoubleClick}
    >
      {/* Matrix rain */}
      {matrixMode && (
        <div className="absolute inset-0 z-5 pointer-events-none overflow-hidden">
          {matrixDrops.map((drop, i) => (
            <div
              key={i}
              className="absolute text-green-500 font-mono text-sm"
              style={{
                left: `${drop.x}%`,
                top: `${drop.y}%`,
                textShadow: '0 0 10px #0f0',
                writingMode: 'vertical-rl',
              }}
            >
              {drop.chars.map((char, j) => (
                <span key={j} style={{ opacity: 1 - j * 0.05 }}>{char}</span>
              ))}
            </div>
          ))}
        </div>
      )}

      {/* Fruit Ninja slash trail */}
      {fruitNinjaMode && slashTrail.length > 1 && (
        <svg className="absolute inset-0 z-15 pointer-events-none" style={{ width: '100%', height: '100%' }}>
          <defs>
            <linearGradient id="slashGradient" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="rgba(255,50,50,0)" />
              <stop offset="50%" stopColor="rgba(255,100,100,0.8)" />
              <stop offset="100%" stopColor="rgba(255,255,255,1)" />
            </linearGradient>
          </defs>
          {slashTrail.slice(1).map((point, i) => {
            const prev = slashTrail[i]
            const age = Date.now() - point.t
            const opacity = Math.max(0, 1 - age / 200)
            return (
              <line
                key={i}
                x1={prev.x}
                y1={prev.y}
                x2={point.x}
                y2={point.y}
                stroke={`rgba(255, ${100 + i * 5}, ${100 + i * 5}, ${opacity})`}
                strokeWidth={3 + (slashTrail.length - i) * 0.3}
                strokeLinecap="round"
              />
            )
          })}
        </svg>
      )}

      {/* Cards with distance-based opacity */}
      <div className="absolute inset-0 z-10">
        {cards.map((card) => {
          const opacity = getCardOpacity(card)
          if (opacity <= 0 && !card.exploding) return null

          return (
            <div
              key={card.id}
              className={`absolute transition-opacity duration-75 ${card.exploding ? 'scale-150' : ''} ${card.frozen ? 'ring-2 ring-cyan-400/50' : ''}`}
              style={{
                left: `${card.x}%`,
                top: `${card.y}%`,
                transform: `translate(-50%, -50%) scale(${card.scale}) rotate(${card.rotation}deg) ${card.flipped ? 'rotateY(180deg)' : ''}`,
                cursor: dragging === card.id ? "grabbing" : "grab",
                zIndex: dragging === card.id ? 100 : 1,
                transformStyle: 'preserve-3d',
                opacity: card.exploding ? 0 : opacity,
              }}
              onMouseDown={(e) => handleMouseDown(e, card.id)}
              onClick={(e) => dragging === null && handleClick(card, e)}
              onDoubleClick={() => handleDoubleClick(card.id)}
            >
              <div
                className="flex flex-col items-center gap-2 p-4 rounded-lg border border-white/20 bg-white/10 shadow-lg shadow-white/5"
                style={{
                  backfaceVisibility: 'hidden',
                  background: card.rainbow ? `hsl(${(Date.now() / 10 + parseInt(card.id.split('-')[1] || '0') * 30) % 360}, 70%, 20%)` : undefined,
                }}
              >
                <i className={`${card.icon} text-3xl text-white`} />
                <span className="text-xs font-mono text-white/90">{card.name}()</span>
              </div>
              <div
                className="absolute inset-0 flex items-center justify-center p-2 rounded-lg border border-white/20 bg-white/10"
                style={{ backfaceVisibility: 'hidden', transform: 'rotateY(180deg)' }}
              >
                <pre className="text-[8px] font-mono text-white/80 whitespace-pre-wrap">{card.code}</pre>
              </div>
            </div>
          )
        })}
      </div>

      {/* Charge indicator when holding right click */}
      {blackHole && (
        <div
          className="fixed z-45 pointer-events-none"
          style={{
            left: mousePos.x,
            top: mousePos.y,
            transform: 'translate(-50%, -50%)',
          }}
        >
          <div
            className="rounded-full border-2 border-white/40"
            style={{
              width: chargeLevel * 2 + 20,
              height: chargeLevel * 2 + 20,
              boxShadow: `0 0 ${chargeLevel / 2}px rgba(255,255,255,0.3)`,
              transition: 'width 0.05s, height 0.05s',
            }}
          />
        </div>
      )}

      {/* Cursor */}
      <div
        className="fixed z-50 pointer-events-none transition-transform"
        style={{
          left: mousePos.x - 12,
          top: mousePos.y - 12,
          width: 24,
          height: 24,
          transform: slowMo ? 'scale(1.5)' : 'scale(1)',
        }}
      >
        <div
          className="w-full h-full rounded-full border-2 border-white/60 bg-white/10"
          style={{
            borderColor: partyMode ? `hsl(${Date.now() / 5 % 360}, 70%, 60%)` : matrixMode ? '#0f0' : undefined,
            boxShadow: slowMo ? '0 0 20px rgba(255,255,255,0.5)' : undefined,
          }}
        />
      </div>

      {/* Vignette */}
      <div className="absolute inset-0 pointer-events-none z-40" style={{ background: "radial-gradient(ellipse at center, transparent 0%, rgba(0,0,0,0.6) 100%)" }} />

      {/* HUD */}
      <div className="fixed bottom-4 left-4 z-50 text-white/30 text-xs font-mono space-y-1">
        {konamiProgress > 0 && <div>Konami: {konamiProgress}/{konamiCode.length}</div>}
        {gravityMode && <div>GRAVITY MODE</div>}
        {matrixMode && <div className="text-green-500">MATRIX MODE</div>}
        {tornado && <div>TORNADO!</div>}
        {slowMo && <div>SLOW MOTION</div>}
        {fruitNinjaMode && <div className="text-red-400">FRUIT NINJA MODE</div>}
      </div>

      {/* Help hint */}
      <div className="fixed bottom-4 right-4 z-50 text-white/20 text-[10px] font-mono text-right">
        <div>Try: gravity, matrix, freeze, boom, sort</div>
        <div>Hold right-click: charge & explode | Space: slow-mo</div>
        <div>Circle mouse: tornado | 3x click: delete</div>
        <div>Swipe left-right 10x: fruit ninja mode</div>
      </div>
    </div>
  )
}
