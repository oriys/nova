"use client"

import { useEffect, useRef, useCallback } from "react"

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  size: number
  alpha: number
  targetAlpha: number
  twinkleSpeed: number
  twinklePhase: number
}

interface ParticleCanvasProps {
  className?: string
  darkMode?: boolean
}

export function ParticleCanvas({ className, darkMode = false }: ParticleCanvasProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const particlesRef = useRef<Particle[]>([])
  const mouseRef = useRef({ x: -1000, y: -1000 })
  const animationRef = useRef<number | undefined>(undefined)

  const createParticle = useCallback((width: number, height: number, x?: number, y?: number): Particle => {
    return {
      x: x ?? Math.random() * width,
      y: y ?? Math.random() * height,
      vx: (Math.random() - 0.5) * 0.3,
      vy: (Math.random() - 0.5) * 0.3,
      size: Math.random() * 2 + 0.5,
      alpha: Math.random() * 0.5 + 0.1,
      targetAlpha: Math.random() * 0.6 + 0.2,
      twinkleSpeed: Math.random() * 0.02 + 0.005,
      twinklePhase: Math.random() * Math.PI * 2,
    }
  }, [])

  const initParticles = useCallback((width: number, height: number) => {
    const particleCount = Math.floor((width * height) / 8000)
    particlesRef.current = Array.from({ length: particleCount }, () =>
      createParticle(width, height)
    )
  }, [createParticle])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const ctx = canvas.getContext("2d")
    if (!ctx) return

    const resize = () => {
      canvas.width = window.innerWidth
      canvas.height = window.innerHeight
      initParticles(canvas.width, canvas.height)
    }

    resize()
    window.addEventListener("resize", resize)

    const handleMouseMove = (e: MouseEvent) => {
      mouseRef.current = { x: e.clientX, y: e.clientY }
    }

    window.addEventListener("mousemove", handleMouseMove)

    // Colors based on mode
    const bgColor = darkMode ? "rgba(0, 0, 0, 1)" : "rgba(255, 255, 255, 1)"
    const particleColor = darkMode ? "255, 255, 255" : "0, 0, 0"
    const lineAlpha = darkMode ? 0.03 : 0.06

    const animate = () => {
      ctx.fillStyle = bgColor
      ctx.fillRect(0, 0, canvas.width, canvas.height)

      const mouseX = mouseRef.current.x
      const mouseY = mouseRef.current.y
      const mouseRadius = 150

      particlesRef.current.forEach((p) => {
        // Twinkle effect
        p.twinklePhase += p.twinkleSpeed
        const twinkle = Math.sin(p.twinklePhase) * 0.3 + 0.7

        // Calculate distance from mouse
        const dx = p.x - mouseX
        const dy = p.y - mouseY
        const dist = Math.sqrt(dx * dx + dy * dy)

        // Mouse repulsion effect
        if (dist < mouseRadius && dist > 0) {
          const force = (mouseRadius - dist) / mouseRadius
          const angle = Math.atan2(dy, dx)
          p.vx += Math.cos(angle) * force * 0.8
          p.vy += Math.sin(angle) * force * 0.8
        }

        // Apply velocity with damping
        p.x += p.vx
        p.y += p.vy
        p.vx *= 0.98
        p.vy *= 0.98

        // Wrap around edges
        if (p.x < 0) p.x = canvas.width
        if (p.x > canvas.width) p.x = 0
        if (p.y < 0) p.y = canvas.height
        if (p.y > canvas.height) p.y = 0

        // Draw particle with twinkle
        const finalAlpha = p.alpha * twinkle
        ctx.beginPath()
        ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2)
        ctx.fillStyle = `rgba(${particleColor}, ${finalAlpha})`
        ctx.fill()

        // Draw glow for brighter particles
        if (finalAlpha > 0.3) {
          ctx.beginPath()
          ctx.arc(p.x, p.y, p.size * 2, 0, Math.PI * 2)
          const gradient = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, p.size * 3)
          gradient.addColorStop(0, `rgba(${particleColor}, ${finalAlpha * 0.3})`)
          gradient.addColorStop(1, `rgba(${particleColor}, 0)`)
          ctx.fillStyle = gradient
          ctx.fill()
        }
      })

      // Draw subtle connecting lines between nearby particles
      ctx.strokeStyle = `rgba(${particleColor}, ${lineAlpha})`
      ctx.lineWidth = 0.5
      for (let i = 0; i < particlesRef.current.length; i++) {
        for (let j = i + 1; j < particlesRef.current.length; j++) {
          const p1 = particlesRef.current[i]
          const p2 = particlesRef.current[j]
          const dx = p1.x - p2.x
          const dy = p1.y - p2.y
          const dist = Math.sqrt(dx * dx + dy * dy)

          if (dist < 100) {
            ctx.beginPath()
            ctx.moveTo(p1.x, p1.y)
            ctx.lineTo(p2.x, p2.y)
            ctx.globalAlpha = (1 - dist / 100) * 0.15
            ctx.stroke()
            ctx.globalAlpha = 1
          }
        }
      }

      animationRef.current = requestAnimationFrame(animate)
    }

    animate()

    return () => {
      window.removeEventListener("resize", resize)
      window.removeEventListener("mousemove", handleMouseMove)
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current)
      }
    }
  }, [initParticles, darkMode])

  return (
    <canvas
      ref={canvasRef}
      className={className}
      style={{
        position: "fixed",
        top: 0,
        left: 0,
        width: "100%",
        height: "100%",
        pointerEvents: "none",
      }}
    />
  )
}
