"use client"

import { useRouter } from "next/navigation"
import { useEffect, useState, useRef, useCallback, useMemo } from "react"
import { useTranslations } from "next-intl"

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
  exploding: boolean
}

interface Particle {
  id: number
  x: number
  y: number
  vx: number
  vy: number
  size: number
  alpha: number
  life: number
}

export default function LandingPage() {
  const t = useTranslations("landingPage")
  const router = useRouter()
  const [mounted, setMounted] = useState(false)
  const [cards, setCards] = useState<FunctionCard[]>([])
  const [dragging, setDragging] = useState<string | null>(null)
  const [particles, setParticles] = useState<Particle[]>([])

  // Use refs for high-frequency updates to avoid React re-renders
  const mousePosRef = useRef({ x: -1000, y: -1000 })
  const [mousePos, setMousePos] = useState({ x: -1000, y: -1000 })
  const circleRef = useRef({ active: false, x: 0, y: 0, radius: 0, maxRadius: 0 })
  const maskContainerRef = useRef<HTMLDivElement>(null)

  const dragOffset = useRef({ x: 0, y: 0 })
  const lastDragPos = useRef({ x: 0, y: 0 })
  const rafId = useRef<number>(0)

  // Initialize cards once
  const initialCards = useMemo(() => {
    const startLang = startLanguages[Math.floor(Math.random() * startLanguages.length)]
    return [
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
        exploding: false,
      })),
      {
        id: 'start',
        lang: startLang.lang,
        icon: startLang.icon,
        name: t("startCard.name"),
        code: `${t("startCard.codeComment")}\nrouter.push("/dashboard")`,
        isStart: true,
        x: Math.random() * 80 + 10,
        y: Math.random() * 75 + 10,
        scale: Math.random() * 0.3 + 0.7,
        rotation: 0,
        flipped: false,
        velocity: { x: 0, y: 0 },
        exploding: false,
      },
    ]
  }, [t])

  useEffect(() => {
    setMounted(true)
    setCards(initialCards)
  }, [initialCards])

  // Unified animation loop using RAF
  useEffect(() => {
    if (!mounted) return

    let lastTime = performance.now()

    const animate = (currentTime: number) => {
      const deltaTime = Math.min((currentTime - lastTime) / 16.67, 2) // Cap at 2x speed
      lastTime = currentTime

      // Update circle transition (direct DOM manipulation for performance)
      if (circleRef.current.active) {
        const speed = Math.max(50, circleRef.current.radius * 0.12)
        circleRef.current.radius += speed * deltaTime

        if (circleRef.current.radius >= circleRef.current.maxRadius) {
          router.push("/dashboard")
          return
        }

        // Direct DOM update for mask (avoids React re-render)
        if (maskContainerRef.current) {
          const { x, y, radius } = circleRef.current
          maskContainerRef.current.style.maskImage = `radial-gradient(circle at ${x}px ${y}px, transparent ${radius}px, black ${radius + 2}px)`
          maskContainerRef.current.style.webkitMaskImage = `radial-gradient(circle at ${x}px ${y}px, transparent ${radius}px, black ${radius + 2}px)`
        }
      }

      // Update cards physics (batched)
      setCards(prev => {
        let hasChanges = false
        const updated = prev.map(card => {
          if (Math.abs(card.velocity.x) < 0.01 && Math.abs(card.velocity.y) < 0.01) return card
          hasChanges = true

          let newVelX = card.velocity.x * (0.95 ** deltaTime)
          let newVelY = card.velocity.y * (0.95 ** deltaTime)
          let newX = card.x + newVelX * deltaTime
          let newY = card.y + newVelY * deltaTime

          if (newX < 5 || newX > 95) newVelX = -newVelX * 0.8
          if (newY < 5 || newY > 95) newVelY = -newVelY * 0.8
          newX = Math.max(5, Math.min(95, newX))
          newY = Math.max(5, Math.min(95, newY))

          return { ...card, x: newX, y: newY, velocity: { x: newVelX, y: newVelY } }
        })
        return hasChanges ? updated : prev
      })

      // Update particles (batched)
      setParticles(prev => {
        if (prev.length === 0) return prev
        const updated = prev
          .map(p => ({
            ...p,
            x: p.x + p.vx * deltaTime,
            y: p.y + p.vy * deltaTime,
            vx: p.vx * (0.98 ** deltaTime),
            vy: p.vy * (0.98 ** deltaTime),
            life: p.life - deltaTime,
            alpha: p.alpha * (0.97 ** deltaTime),
            size: p.size * (0.995 ** deltaTime),
          }))
          .filter(p => p.life > 0 && p.alpha > 0.01)
        return updated
      })

      rafId.current = requestAnimationFrame(animate)
    }

    rafId.current = requestAnimationFrame(animate)
    return () => cancelAnimationFrame(rafId.current)
  }, [mounted, router])

  // Throttled mouse position update for card opacity
  const handleMouseMove = useCallback((e: MouseEvent) => {
    mousePosRef.current = { x: e.clientX, y: e.clientY }

    // Throttle React state updates to ~30fps for opacity calculations
    if (!rafId.current) return
    setMousePos({ x: e.clientX, y: e.clientY })

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
  }, [dragging])

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
  }, [dragging])

  useEffect(() => {
    window.addEventListener("mousemove", handleMouseMove, { passive: true })
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

  const handleDoubleClick = (cardId: string) => {
    setCards(prev => prev.map(card => card.id === cardId ? { ...card, flipped: !card.flipped } : card))
  }

  const handleTripleClick = (cardId: string) => {
    const card = cards.find(c => c.id === cardId)
    if (!card) return

    const cx = (card.x / 100) * window.innerWidth
    const cy = (card.y / 100) * window.innerHeight
    const newParticles: Particle[] = Array.from({ length: 80 }, (_, i) => ({
      id: Date.now() + i,
      x: cx + (Math.random() - 0.5) * 80,
      y: cy + (Math.random() - 0.5) * 60,
      vx: (Math.random() - 0.5) * 1.5,
      vy: -Math.random() * 1.5 - 0.3,
      size: Math.random() * 3 + 1,
      alpha: Math.random() * 0.6 + 0.4,
      life: 50 + Math.random() * 40,
    }))
    setParticles(prev => [...prev, ...newParticles])

    setCards(prev => prev.map(c => c.id === cardId ? { ...c, exploding: true } : c))
    setTimeout(() => setCards(prev => prev.filter(c => c.id !== cardId)), 300)
  }

  const handleClick = (card: FunctionCard, e: React.MouseEvent) => {
    if (e.detail === 3) {
      handleTripleClick(card.id)
      return
    }
    if (card.isStart && !circleRef.current.active) {
      const maxDist = Math.sqrt(
        Math.pow(Math.max(e.clientX, window.innerWidth - e.clientX), 2) +
        Math.pow(Math.max(e.clientY, window.innerHeight - e.clientY), 2)
      )
      // Use ref for animation, avoid state updates every frame
      circleRef.current = {
        active: true,
        x: e.clientX,
        y: e.clientY,
        radius: 0,
        maxRadius: maxDist + 100,
      }
    }
  }

  // Memoized opacity calculation
  const getCardOpacity = useCallback((card: FunctionCard) => {
    const cardPixelX = (card.x / 100) * window.innerWidth
    const cardPixelY = (card.y / 100) * window.innerHeight
    const dx = cardPixelX - mousePos.x
    const dy = cardPixelY - mousePos.y
    const dist = Math.sqrt(dx * dx + dy * dy)
    return Math.max(0, 1 - dist / 300)
  }, [mousePos.x, mousePos.y])

  if (!mounted || cards.length === 0) {
    return <div className="h-screen w-screen bg-black" />
  }

  return (
    <>
      {/* Bottom layer: Dashboard (always mounted for preload) */}
      <iframe
        src="/dashboard"
        className="fixed inset-0 w-full h-full border-0"
        style={{ zIndex: 0, transform: 'translateZ(0)' }}
      />

      {/* Top layer: Landing page with mask */}
      <div
        ref={maskContainerRef}
        className="relative h-screen w-screen overflow-hidden bg-black cursor-none select-none"
        onContextMenu={(e) => e.preventDefault()}
        style={{
          zIndex: 1,
          willChange: circleRef.current.active ? 'mask-image' : 'auto',
          transform: 'translateZ(0)', // Force GPU layer
        }}
      >
        {/* Cards */}
        <div className="absolute inset-0 z-10">
          {cards.map((card) => {
            const opacity = getCardOpacity(card)
            if (opacity <= 0 && !card.exploding) return null

            return (
              <div
                key={card.id}
                className={`absolute ${card.exploding ? 'scale-150' : ''}`}
                style={{
                  left: `${card.x}%`,
                  top: `${card.y}%`,
                  transform: `translate(-50%, -50%) scale(${card.scale}) rotate(${card.rotation}deg) ${card.flipped ? 'rotateY(180deg)' : ''}`,
                  cursor: dragging === card.id ? "grabbing" : "grab",
                  zIndex: dragging === card.id ? 100 : 1,
                  transformStyle: 'preserve-3d',
                  opacity: card.exploding ? 0 : opacity,
                  willChange: 'transform, opacity',
                  transition: card.exploding ? 'opacity 0.3s, transform 0.3s' : 'opacity 0.075s',
                }}
                onMouseDown={(e) => handleMouseDown(e, card.id)}
                onClick={(e) => dragging === null && handleClick(card, e)}
                onDoubleClick={() => handleDoubleClick(card.id)}
              >
                <div
                  className="flex flex-col items-center gap-2 p-4 rounded-lg border border-white/20 bg-white/10 shadow-lg shadow-white/5"
                  style={{ backfaceVisibility: 'hidden' }}
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

        {/* Particles (GPU accelerated) */}
        {particles.length > 0 && (
          <div className="absolute inset-0 z-20 pointer-events-none">
            {particles.map(p => (
              <div
                key={p.id}
                className="absolute rounded-full bg-white"
                style={{
                  left: p.x,
                  top: p.y,
                  width: p.size,
                  height: p.size,
                  opacity: p.alpha,
                  boxShadow: `0 0 ${p.size * 2}px rgba(255,255,255,${p.alpha * 0.5})`,
                  transform: 'translate(-50%, -50%) translateZ(0)',
                }}
              />
            ))}
          </div>
        )}

        {/* Cursor */}
        <div
          className="fixed z-50 pointer-events-none"
          style={{
            left: mousePos.x - 12,
            top: mousePos.y - 12,
            width: 24,
            height: 24,
            transform: 'translateZ(0)',
          }}
        >
          <div className="w-full h-full rounded-full border-2 border-white/60 bg-white/10" />
        </div>

        {/* Vignette */}
        <div
          className="absolute inset-0 pointer-events-none z-40"
          style={{ background: "radial-gradient(ellipse at center, transparent 0%, rgba(0,0,0,0.6) 100%)" }}
        />
      </div>
    </>
  )
}
