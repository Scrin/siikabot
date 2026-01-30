import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { ShaderPass } from 'three/examples/jsm/postprocessing/ShaderPass.js'

// Chromatic aberration shader
const ChromaticAberrationShader = {
  uniforms: {
    tDiffuse: { value: null },
    amount: { value: 0.003 },
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
    varying vec2 vUv;
    void main() {
      vec2 offset = amount * vec2(1.0, 0.0);
      float r = texture2D(tDiffuse, vUv + offset).r;
      float g = texture2D(tDiffuse, vUv).g;
      float b = texture2D(tDiffuse, vUv - offset).b;
      gl_FragColor = vec4(r, g, b, 1.0);
    }
  `,
}

export function RetroBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const mouseRef = useRef({ x: 0, y: 0, targetX: 0, targetY: 0 })

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

    // Bloom pass for neon glow
    const bloomPass = new UnrealBloomPass(
      new THREE.Vector2(window.innerWidth, window.innerHeight),
      0.6, // strength
      0.4, // radius
      0.85, // threshold
    )
    composer.addPass(bloomPass)

    // Chromatic aberration
    const chromaticPass = new ShaderPass(ChromaticAberrationShader)
    composer.addPass(chromaticPass)

    // Colors matching the theme
    const purpleColor = new THREE.Color(0x8b5cf6)
    const blueColor = new THREE.Color(0x3b82f6)
    const cyanColor = new THREE.Color(0x06b6d4)

    // Create wireframe geometries
    const pyramidGeometry = new THREE.ConeGeometry(10, 15, 4)
    const pyramidMaterial = new THREE.MeshBasicMaterial({
      color: purpleColor,
      wireframe: true,
      transparent: true,
      opacity: 0.4,
    })
    const pyramid = new THREE.Mesh(pyramidGeometry, pyramidMaterial)
    pyramid.position.x = -15
    scene.add(pyramid)

    // Cube
    const cubeGeometry = new THREE.BoxGeometry(12, 12, 12)
    const cubeMaterial = new THREE.MeshBasicMaterial({
      color: blueColor,
      wireframe: true,
      transparent: true,
      opacity: 0.35,
    })
    const cube = new THREE.Mesh(cubeGeometry, cubeMaterial)
    cube.position.x = 15
    cube.position.y = -10
    scene.add(cube)

    // Icosahedron (retro polyhedron)
    const icosahedronGeometry = new THREE.IcosahedronGeometry(8, 0)
    const icosahedronMaterial = new THREE.MeshBasicMaterial({
      color: cyanColor,
      wireframe: true,
      transparent: true,
      opacity: 0.4,
    })
    const icosahedron = new THREE.Mesh(icosahedronGeometry, icosahedronMaterial)
    icosahedron.position.y = 15
    scene.add(icosahedron)

    // Torus knot for extra flair
    const torusKnotGeometry = new THREE.TorusKnotGeometry(5, 1.5, 100, 16)
    const torusKnotMaterial = new THREE.MeshBasicMaterial({
      color: new THREE.Color(0xec4899), // pink
      wireframe: true,
      transparent: true,
      opacity: 0.25,
    })
    const torusKnot = new THREE.Mesh(torusKnotGeometry, torusKnotMaterial)
    torusKnot.position.set(-20, -15, -10)
    scene.add(torusKnot)

    // Octahedron
    const octahedronGeometry = new THREE.OctahedronGeometry(6)
    const octahedronMaterial = new THREE.MeshBasicMaterial({
      color: new THREE.Color(0x10b981), // emerald
      wireframe: true,
      transparent: true,
      opacity: 0.3,
    })
    const octahedron = new THREE.Mesh(octahedronGeometry, octahedronMaterial)
    octahedron.position.set(25, 10, -15)
    scene.add(octahedron)

    // Enhanced particle field with velocities
    const particleCount = 2500
    const particlesGeometry = new THREE.BufferGeometry()
    const positions = new Float32Array(particleCount * 3)
    const colors = new Float32Array(particleCount * 3)
    const velocities = new Float32Array(particleCount * 3)

    for (let i = 0; i < particleCount; i++) {
      const i3 = i * 3

      // Random positions in a large sphere
      positions[i3] = (Math.random() - 0.5) * 120
      positions[i3 + 1] = (Math.random() - 0.5) * 120
      positions[i3 + 2] = (Math.random() - 0.5) * 120

      // Velocities for flowing movement
      velocities[i3] = (Math.random() - 0.5) * 0.02
      velocities[i3 + 1] = (Math.random() - 0.5) * 0.02
      velocities[i3 + 2] = (Math.random() - 0.5) * 0.02

      // Random colors between purple, blue, and cyan
      const colorChoice = Math.random()
      if (colorChoice > 0.66) {
        colors[i3] = purpleColor.r
        colors[i3 + 1] = purpleColor.g
        colors[i3 + 2] = purpleColor.b
      } else if (colorChoice > 0.33) {
        colors[i3] = blueColor.r
        colors[i3 + 1] = blueColor.g
        colors[i3 + 2] = blueColor.b
      } else {
        colors[i3] = cyanColor.r
        colors[i3 + 1] = cyanColor.g
        colors[i3 + 2] = cyanColor.b
      }
    }

    particlesGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
    particlesGeometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))

    const particlesMaterial = new THREE.PointsMaterial({
      size: 0.2,
      vertexColors: true,
      transparent: true,
      opacity: 0.7,
      blending: THREE.AdditiveBlending,
      sizeAttenuation: true,
    })

    const particles = new THREE.Points(particlesGeometry, particlesMaterial)
    scene.add(particles)

    // Grid helper for retro look
    const gridHelper = new THREE.GridHelper(100, 50, purpleColor, blueColor)
    gridHelper.material.transparent = true
    gridHelper.material.opacity = 0.15
    gridHelper.position.y = -20
    scene.add(gridHelper)

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

      // Smooth mouse interpolation
      mouseRef.current.x += (mouseRef.current.targetX - mouseRef.current.x) * 0.05
      mouseRef.current.y += (mouseRef.current.targetY - mouseRef.current.y) * 0.05

      // Apply mouse to camera for parallax
      camera.position.x = mouseRef.current.x * 5
      camera.position.y = mouseRef.current.y * 3
      camera.lookAt(scene.position)

      // Rotate geometries with parallax offset
      pyramid.rotation.x += 0.003
      pyramid.rotation.y += 0.005
      pyramid.position.x = -15 + mouseRef.current.x * 3

      cube.rotation.x += 0.002
      cube.rotation.y += 0.003
      cube.rotation.z += 0.001
      cube.position.y = -10 + mouseRef.current.y * 2

      icosahedron.rotation.x += 0.004
      icosahedron.rotation.y += 0.002
      icosahedron.position.y = 15 + mouseRef.current.y * 2
      icosahedron.position.x = mouseRef.current.x * 2

      torusKnot.rotation.x += 0.003
      torusKnot.rotation.y += 0.004
      torusKnot.position.x = -20 + mouseRef.current.x * 1.5

      octahedron.rotation.x += 0.002
      octahedron.rotation.z += 0.003
      octahedron.position.y = 10 + mouseRef.current.y * 1.5

      // Animate particles - flowing movement
      const positionArray = particlesGeometry.attributes.position.array as Float32Array
      for (let i = 0; i < particleCount * 3; i += 3) {
        positionArray[i] += velocities[i]
        positionArray[i + 1] += velocities[i + 1]
        positionArray[i + 2] += velocities[i + 2]

        // Boundary wrapping
        if (Math.abs(positionArray[i]) > 60) velocities[i] *= -1
        if (Math.abs(positionArray[i + 1]) > 60) velocities[i + 1] *= -1
        if (Math.abs(positionArray[i + 2]) > 60) velocities[i + 2] *= -1
      }
      particlesGeometry.attributes.position.needsUpdate = true

      // Rotate particle field slowly
      particles.rotation.y += 0.0003

      // Subtle pulsing on grid opacity
      if (Array.isArray(gridHelper.material)) {
        gridHelper.material.forEach((mat) => {
          mat.opacity = 0.1 + Math.sin(time) * 0.05
        })
      } else {
        gridHelper.material.opacity = 0.1 + Math.sin(time) * 0.05
      }

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
      particlesGeometry.dispose()
      particlesMaterial.dispose()
      gridHelper.geometry.dispose()
      if (Array.isArray(gridHelper.material)) {
        gridHelper.material.forEach((mat) => mat.dispose())
      } else {
        gridHelper.material.dispose()
      }
    }
  }, [])

  return (
    <canvas ref={canvasRef} className="fixed inset-0 h-full w-full" style={{ zIndex: 0 }} />
  )
}
