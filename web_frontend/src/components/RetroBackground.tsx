import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'
import { FilmPass } from 'three/examples/jsm/postprocessing/FilmPass.js'
import { AfterimagePass } from 'three/examples/jsm/postprocessing/AfterimagePass.js'

// Chromatic aberration shader
const ChromaticAberrationShader = {
  uniforms: {
    tDiffuse: { value: null },
    amount: { value: 0.005 },
    time: { value: 0 },
  },
  vertexShader: `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: `
    uniform sampler2D tDiffuse;
    uniform float amount;
    uniform float time;
    varying vec2 vUv;
    void main() {
      vec2 offset = amount * vec2(1.0 + sin(time * 2.0) * 0.3, 0.0);
      float r = texture2D(tDiffuse, vUv + offset).r;
      float g = texture2D(tDiffuse, vUv).g;
      float b = texture2D(tDiffuse, vUv - offset).b;
      gl_FragColor = vec4(r, g, b, 1.0);
    }
  `,
}

// Vignette shader
const VignetteShader = {
  uniforms: {
    tDiffuse: { value: null },
    darkness: { value: 0.6 },
    offset: { value: 1.0 },
  },
  vertexShader: `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: `
    uniform sampler2D tDiffuse;
    uniform float darkness;
    uniform float offset;
    varying vec2 vUv;
    void main() {
      vec4 texel = texture2D(tDiffuse, vUv);
      vec2 uv = (vUv - vec2(0.5)) * vec2(offset);
      float vignette = 1.0 - dot(uv, uv);
      texel.rgb *= smoothstep(0.0, 1.0, vignette * (1.0 + darkness));
      gl_FragColor = texel;
    }
  `,
}

// Create DNA helix geometry
function createDNAHelix(): THREE.BufferGeometry {
  const points: THREE.Vector3[] = []
  const turns = 3
  const pointsPerTurn = 50
  const totalPoints = turns * pointsPerTurn
  const radius = 3
  const height = 20

  for (let i = 0; i <= totalPoints; i++) {
    const t = i / totalPoints
    const angle = t * turns * Math.PI * 2
    const y = (t - 0.5) * height

    // First strand
    points.push(new THREE.Vector3(Math.cos(angle) * radius, y, Math.sin(angle) * radius))
  }

  // Second strand (offset by PI)
  for (let i = 0; i <= totalPoints; i++) {
    const t = i / totalPoints
    const angle = t * turns * Math.PI * 2 + Math.PI
    const y = (t - 0.5) * height
    points.push(new THREE.Vector3(Math.cos(angle) * radius, y, Math.sin(angle) * radius))
  }

  // Connecting rungs
  for (let i = 0; i <= totalPoints; i += 5) {
    const t = i / totalPoints
    const angle = t * turns * Math.PI * 2
    const y = (t - 0.5) * height
    points.push(new THREE.Vector3(Math.cos(angle) * radius, y, Math.sin(angle) * radius))
    points.push(
      new THREE.Vector3(Math.cos(angle + Math.PI) * radius, y, Math.sin(angle + Math.PI) * radius),
    )
  }

  const geometry = new THREE.BufferGeometry().setFromPoints(points)
  return geometry
}

interface RetroBackgroundProps {
  reducedEffects?: boolean
}

export function RetroBackground({ reducedEffects = false }: RetroBackgroundProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const mouseRef = useRef({ x: 0, y: 0, targetX: 0, targetY: 0, speed: 0, prevX: 0, prevY: 0 })
  const reducedEffectsRef = useRef(reducedEffects)

  // Keep the ref in sync with the prop
  reducedEffectsRef.current = reducedEffects

  useEffect(() => {
    if (!canvasRef.current) return

    // Scene setup
    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(
      75,
      window.innerWidth / window.innerHeight,
      0.1,
      1000,
    )
    camera.position.z = 30

    const renderer = new THREE.WebGLRenderer({
      canvas: canvasRef.current,
      alpha: true,
      antialias: true,
    })
    renderer.setSize(window.innerWidth, window.innerHeight)
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))

    // Post-processing setup
    const composer = new EffectComposer(renderer)
    const renderPass = new RenderPass(scene, camera)
    composer.addPass(renderPass)

    // Afterimage pass for motion trails
    const afterimagePass = new AfterimagePass(0.92)
    composer.addPass(afterimagePass)

    // Bloom pass for neon glow (dynamic)
    const bloomPass = new UnrealBloomPass(
      new THREE.Vector2(window.innerWidth, window.innerHeight),
      0.6,
      0.4,
      0.85,
    )
    composer.addPass(bloomPass)

    // Film grain pass
    const filmPass = new FilmPass(0.2, false)
    composer.addPass(filmPass)

    // Chromatic aberration
    const chromaticPass = new ShaderPass(ChromaticAberrationShader)
    composer.addPass(chromaticPass)

    // Vignette pass
    const vignettePass = new ShaderPass(VignetteShader)
    composer.addPass(vignettePass)

    // Colors matching the theme
    const purpleColor = new THREE.Color(0x8b5cf6)
    const blueColor = new THREE.Color(0x3b82f6)
    const cyanColor = new THREE.Color(0x06b6d4)
    const pinkColor = new THREE.Color(0xec4899)
    const emeraldColor = new THREE.Color(0x10b981)
    const orangeColor = new THREE.Color(0xf97316)
    const violetColor = new THREE.Color(0x7c3aed)
    const yellowGreenColor = new THREE.Color(0x84cc16)

    // === GEOMETRIC SHAPES ===

    // 1. Pyramid (existing)
    const pyramidGeometry = new THREE.ConeGeometry(10, 15, 4)
    const pyramidMaterial = new THREE.MeshBasicMaterial({
      color: purpleColor,
      wireframe: true,
      transparent: true,
      opacity: 0.4,
    })
    const pyramid = new THREE.Mesh(pyramidGeometry, pyramidMaterial)
    pyramid.position.set(-15, 0, 0)
    scene.add(pyramid)

    // 2. Cube (existing)
    const cubeGeometry = new THREE.BoxGeometry(12, 12, 12)
    const cubeMaterial = new THREE.MeshBasicMaterial({
      color: blueColor,
      wireframe: true,
      transparent: true,
      opacity: 0.35,
    })
    const cube = new THREE.Mesh(cubeGeometry, cubeMaterial)
    cube.position.set(15, -10, 0)
    scene.add(cube)

    // 3. Icosahedron (existing)
    const icosahedronGeometry = new THREE.IcosahedronGeometry(8, 0)
    const icosahedronMaterial = new THREE.MeshBasicMaterial({
      color: cyanColor,
      wireframe: true,
      transparent: true,
      opacity: 0.4,
    })
    const icosahedron = new THREE.Mesh(icosahedronGeometry, icosahedronMaterial)
    icosahedron.position.set(0, 15, 0)
    scene.add(icosahedron)

    // 4. Torus Knot (existing)
    const torusKnotGeometry = new THREE.TorusKnotGeometry(5, 1.5, 100, 16)
    const torusKnotMaterial = new THREE.MeshBasicMaterial({
      color: pinkColor,
      wireframe: true,
      transparent: true,
      opacity: 0.25,
    })
    const torusKnot = new THREE.Mesh(torusKnotGeometry, torusKnotMaterial)
    torusKnot.position.set(-20, -15, -10)
    scene.add(torusKnot)

    // 5. Octahedron (existing)
    const octahedronGeometry = new THREE.OctahedronGeometry(6)
    const octahedronMaterial = new THREE.MeshBasicMaterial({
      color: emeraldColor,
      wireframe: true,
      transparent: true,
      opacity: 0.3,
    })
    const octahedron = new THREE.Mesh(octahedronGeometry, octahedronMaterial)
    octahedron.position.set(25, 10, -15)
    scene.add(octahedron)

    // 6. NEW: Dodecahedron
    const dodecahedronGeometry = new THREE.DodecahedronGeometry(7)
    const dodecahedronMaterial = new THREE.MeshBasicMaterial({
      color: yellowGreenColor,
      wireframe: true,
      transparent: true,
      opacity: 0.35,
    })
    const dodecahedron = new THREE.Mesh(dodecahedronGeometry, dodecahedronMaterial)
    dodecahedron.position.set(-25, 12, -8)
    scene.add(dodecahedron)

    // 7. NEW: Tetrahedron
    const tetrahedronGeometry = new THREE.TetrahedronGeometry(8)
    const tetrahedronMaterial = new THREE.MeshBasicMaterial({
      color: orangeColor,
      wireframe: true,
      transparent: true,
      opacity: 0.4,
    })
    const tetrahedron = new THREE.Mesh(tetrahedronGeometry, tetrahedronMaterial)
    tetrahedron.position.set(30, -5, -12)
    scene.add(tetrahedron)

    // 8. NEW: Torus
    const torusGeometry = new THREE.TorusGeometry(6, 2, 16, 32)
    const torusMaterial = new THREE.MeshBasicMaterial({
      color: violetColor,
      wireframe: true,
      transparent: true,
      opacity: 0.35,
    })
    const torus = new THREE.Mesh(torusGeometry, torusMaterial)
    torus.position.set(0, -20, -5)
    scene.add(torus)

    // 9. NEW: DNA Helix
    const dnaGeometry = createDNAHelix()
    const dnaMaterial = new THREE.LineBasicMaterial({
      color: new THREE.Color(0xffffff),
      transparent: true,
      opacity: 0.5,
    })
    const dna = new THREE.LineSegments(dnaGeometry, dnaMaterial)
    dna.position.set(-35, 0, -20)
    scene.add(dna)

    // 10. NEW: Energy Rings (multiple pulsing rings)
    const ringGeometries: THREE.RingGeometry[] = []
    const ringMeshes: THREE.Mesh[] = []
    for (let i = 0; i < 5; i++) {
      const ringGeometry = new THREE.RingGeometry(8 + i * 3, 8.5 + i * 3, 64)
      const ringMaterial = new THREE.MeshBasicMaterial({
        color: cyanColor,
        wireframe: false,
        transparent: true,
        opacity: 0.2 - i * 0.03,
        side: THREE.DoubleSide,
      })
      const ring = new THREE.Mesh(ringGeometry, ringMaterial)
      ring.position.set(35, 15, -25)
      ring.rotation.x = Math.PI / 2
      scene.add(ring)
      ringGeometries.push(ringGeometry)
      ringMeshes.push(ring)
    }

    // === MASSIVE PARTICLE SYSTEM ===
    const particleCount = 5000
    const particlesGeometry = new THREE.BufferGeometry()
    const positions = new Float32Array(particleCount * 3)
    const colors = new Float32Array(particleCount * 3)
    const velocities = new Float32Array(particleCount * 3)
    const sizes = new Float32Array(particleCount)
    const originalPositions = new Float32Array(particleCount * 3)

    for (let i = 0; i < particleCount; i++) {
      const i3 = i * 3

      // Random positions in a large sphere
      positions[i3] = (Math.random() - 0.5) * 150
      positions[i3 + 1] = (Math.random() - 0.5) * 150
      positions[i3 + 2] = (Math.random() - 0.5) * 150

      // Store original positions for attraction reset
      originalPositions[i3] = positions[i3]
      originalPositions[i3 + 1] = positions[i3 + 1]
      originalPositions[i3 + 2] = positions[i3 + 2]

      // Velocities for flowing movement
      velocities[i3] = (Math.random() - 0.5) * 0.03
      velocities[i3 + 1] = (Math.random() - 0.5) * 0.03
      velocities[i3 + 2] = (Math.random() - 0.5) * 0.03

      // Size based on depth (for depth effect)
      sizes[i] = Math.random() * 0.3 + 0.1

      // Random colors with hue variation over time
      const colorChoice = Math.random()
      if (colorChoice > 0.75) {
        colors[i3] = purpleColor.r
        colors[i3 + 1] = purpleColor.g
        colors[i3 + 2] = purpleColor.b
      } else if (colorChoice > 0.5) {
        colors[i3] = blueColor.r
        colors[i3 + 1] = blueColor.g
        colors[i3 + 2] = blueColor.b
      } else if (colorChoice > 0.25) {
        colors[i3] = cyanColor.r
        colors[i3 + 1] = cyanColor.g
        colors[i3 + 2] = cyanColor.b
      } else {
        colors[i3] = pinkColor.r
        colors[i3 + 1] = pinkColor.g
        colors[i3 + 2] = pinkColor.b
      }
    }

    particlesGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
    particlesGeometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))
    particlesGeometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1))

    const particlesMaterial = new THREE.PointsMaterial({
      size: 0.25,
      vertexColors: true,
      transparent: true,
      opacity: 0.8,
      blending: THREE.AdditiveBlending,
      sizeAttenuation: true,
    })

    const particles = new THREE.Points(particlesGeometry, particlesMaterial)
    scene.add(particles)

    // === GRIDS ===

    // Floor grid
    const gridHelper = new THREE.GridHelper(150, 75, purpleColor, blueColor)
    gridHelper.material.transparent = true
    gridHelper.material.opacity = 0.15
    gridHelper.position.y = -30
    scene.add(gridHelper)

    // Ceiling grid (mirror)
    const ceilingGrid = new THREE.GridHelper(150, 75, cyanColor, pinkColor)
    ceilingGrid.material.transparent = true
    ceilingGrid.material.opacity = 0.1
    ceilingGrid.position.y = 50
    scene.add(ceilingGrid)

    // Mouse tracking
    const handleMouseMove = (event: MouseEvent) => {
      mouseRef.current.targetX = (event.clientX / window.innerWidth) * 2 - 1
      mouseRef.current.targetY = -(event.clientY / window.innerHeight) * 2 + 1
    }
    window.addEventListener('mousemove', handleMouseMove)

    // Animation
    let animationFrameId: number
    let time = 0

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate)
      time += 0.01

      // Calculate mouse speed for dynamic effects
      const dx = mouseRef.current.targetX - mouseRef.current.prevX
      const dy = mouseRef.current.targetY - mouseRef.current.prevY
      mouseRef.current.speed = Math.sqrt(dx * dx + dy * dy)
      mouseRef.current.prevX = mouseRef.current.targetX
      mouseRef.current.prevY = mouseRef.current.targetY

      // Smooth mouse interpolation
      mouseRef.current.x += (mouseRef.current.targetX - mouseRef.current.x) * 0.05
      mouseRef.current.y += (mouseRef.current.targetY - mouseRef.current.y) * 0.05

      // Apply mouse to camera for parallax
      camera.position.x = mouseRef.current.x * 8
      camera.position.y = mouseRef.current.y * 5
      camera.lookAt(scene.position)

      // === ANIMATE SHAPES ===

      // Original shapes
      pyramid.rotation.x += 0.003
      pyramid.rotation.y += 0.005
      pyramid.position.x = -15 + mouseRef.current.x * 4

      cube.rotation.x += 0.002
      cube.rotation.y += 0.003
      cube.rotation.z += 0.001
      cube.position.y = -10 + mouseRef.current.y * 3

      icosahedron.rotation.x += 0.004
      icosahedron.rotation.y += 0.002
      icosahedron.position.y = 15 + mouseRef.current.y * 3
      icosahedron.position.x = mouseRef.current.x * 3

      torusKnot.rotation.x += 0.003
      torusKnot.rotation.y += 0.004
      torusKnot.position.x = -20 + mouseRef.current.x * 2

      octahedron.rotation.x += 0.002
      octahedron.rotation.z += 0.003
      octahedron.position.y = 10 + mouseRef.current.y * 2

      // New shapes
      dodecahedron.rotation.x += 0.002
      dodecahedron.rotation.y += 0.003
      dodecahedron.rotation.z += 0.001
      dodecahedron.position.x = -25 + mouseRef.current.x * 2

      tetrahedron.rotation.x += 0.004
      tetrahedron.rotation.y += 0.003
      tetrahedron.position.y = -5 + mouseRef.current.y * 2

      torus.rotation.x += 0.003
      torus.rotation.z += 0.002
      torus.position.y = -20 + Math.sin(time) * 2

      dna.rotation.y += 0.005
      dna.position.y = Math.sin(time * 0.5) * 3

      // Energy rings pulsing
      ringMeshes.forEach((ring, i) => {
        const scale = 1 + Math.sin(time * 2 + i * 0.5) * 0.15
        ring.scale.set(scale, scale, 1)
        ring.rotation.z = time * 0.2 + i * 0.2
        const material = ring.material as THREE.MeshBasicMaterial
        material.opacity = (0.2 - i * 0.03) * (0.7 + Math.sin(time * 3 + i) * 0.3)
      })

      // === ANIMATE PARTICLES with mouse attraction ===
      const positionArray = particlesGeometry.attributes.position.array as Float32Array
      const colorArray = particlesGeometry.attributes.color.array as Float32Array
      const mouseX = mouseRef.current.x * 50
      const mouseY = mouseRef.current.y * 30

      for (let i = 0; i < particleCount; i++) {
        const i3 = i * 3

        // Apply velocity
        positionArray[i3] += velocities[i3]
        positionArray[i3 + 1] += velocities[i3 + 1]
        positionArray[i3 + 2] += velocities[i3 + 2]

        // Mouse attraction force
        const dx = mouseX - positionArray[i3]
        const dy = mouseY - positionArray[i3 + 1]
        const distance = Math.sqrt(dx * dx + dy * dy)
        if (distance < 40) {
          const strength = (1 - distance / 40) * 0.02
          velocities[i3] += dx * strength * 0.01
          velocities[i3 + 1] += dy * strength * 0.01
        }

        // Damping
        velocities[i3] *= 0.999
        velocities[i3 + 1] *= 0.999
        velocities[i3 + 2] *= 0.999

        // Boundary wrapping
        if (Math.abs(positionArray[i3]) > 75) velocities[i3] *= -1
        if (Math.abs(positionArray[i3 + 1]) > 75) velocities[i3 + 1] *= -1
        if (Math.abs(positionArray[i3 + 2]) > 75) velocities[i3 + 2] *= -1

        // Subtle color cycling (hue shift)
        const hueShift = Math.sin(time * 0.5 + i * 0.001) * 0.1
        colorArray[i3] = Math.min(1, colorArray[i3] + hueShift * 0.01)
        colorArray[i3 + 2] = Math.min(1, colorArray[i3 + 2] - hueShift * 0.01)
      }
      particlesGeometry.attributes.position.needsUpdate = true
      particlesGeometry.attributes.color.needsUpdate = true

      // Rotate particle field slowly
      particles.rotation.y += 0.0002
      particles.rotation.x += 0.0001

      // Hide particles in reduced effects mode
      particles.visible = !reducedEffectsRef.current

      // === DYNAMIC GRID ANIMATION ===
      const gridWave = Math.sin(time * 0.5) * 0.05
      if (Array.isArray(gridHelper.material)) {
        gridHelper.material.forEach((mat) => {
          mat.opacity = 0.1 + gridWave
        })
      } else {
        gridHelper.material.opacity = 0.1 + gridWave
      }

      if (Array.isArray(ceilingGrid.material)) {
        ceilingGrid.material.forEach((mat) => {
          mat.opacity = 0.08 + gridWave * 0.5
        })
      } else {
        ceilingGrid.material.opacity = 0.08 + gridWave * 0.5
      }

      // === DYNAMIC BLOOM ===
      const targetBloom = 0.4 + mouseRef.current.speed * 3 + Math.sin(time * 0.5) * 0.2
      bloomPass.strength = THREE.MathUtils.lerp(bloomPass.strength, targetBloom, 0.1)

      // Update chromatic aberration time
      chromaticPass.uniforms.time.value = time

      // Render with post-processing
      composer.render()
    }

    animate()

    // Handle resize
    const handleResize = () => {
      camera.aspect = window.innerWidth / window.innerHeight
      camera.updateProjectionMatrix()
      renderer.setSize(window.innerWidth, window.innerHeight)
      composer.setSize(window.innerWidth, window.innerHeight)
      bloomPass.setSize(window.innerWidth, window.innerHeight)
    }

    window.addEventListener('resize', handleResize)

    // Pause when tab is not visible
    const handleVisibilityChange = () => {
      if (document.hidden) {
        cancelAnimationFrame(animationFrameId)
      } else {
        animate()
      }
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)

    // Cleanup
    return () => {
      window.removeEventListener('resize', handleResize)
      window.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      cancelAnimationFrame(animationFrameId)
      renderer.dispose()
      composer.dispose()

      // Dispose geometries and materials
      pyramidGeometry.dispose()
      pyramidMaterial.dispose()
      cubeGeometry.dispose()
      cubeMaterial.dispose()
      icosahedronGeometry.dispose()
      icosahedronMaterial.dispose()
      torusKnotGeometry.dispose()
      torusKnotMaterial.dispose()
      octahedronGeometry.dispose()
      octahedronMaterial.dispose()
      dodecahedronGeometry.dispose()
      dodecahedronMaterial.dispose()
      tetrahedronGeometry.dispose()
      tetrahedronMaterial.dispose()
      torusGeometry.dispose()
      torusMaterial.dispose()
      dnaGeometry.dispose()
      dnaMaterial.dispose()
      ringGeometries.forEach((g) => g.dispose())
      ringMeshes.forEach((m) => (m.material as THREE.Material).dispose())
      particlesGeometry.dispose()
      particlesMaterial.dispose()
      gridHelper.geometry.dispose()
      if (Array.isArray(gridHelper.material)) {
        gridHelper.material.forEach((mat) => mat.dispose())
      } else {
        gridHelper.material.dispose()
      }
      ceilingGrid.geometry.dispose()
      if (Array.isArray(ceilingGrid.material)) {
        ceilingGrid.material.forEach((mat) => mat.dispose())
      } else {
        ceilingGrid.material.dispose()
      }
    }
  }, [])

  return (
    <canvas ref={canvasRef} className="fixed inset-0 h-full w-full" style={{ zIndex: 0 }} />
  )
}
