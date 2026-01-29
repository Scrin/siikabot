import { useEffect, useRef } from 'react'
import * as THREE from 'three'

export function RetroBackground() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

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

    // Colors matching the theme
    const purpleColor = new THREE.Color(0x8b5cf6) // purple-500
    const blueColor = new THREE.Color(0x3b82f6) // blue-500
    const cyanColor = new THREE.Color(0x06b6d4) // cyan-500

    // Create wireframe geometries
    const pyramidGeometry = new THREE.ConeGeometry(10, 15, 4)
    const pyramidMaterial = new THREE.MeshBasicMaterial({
      color: purpleColor,
      wireframe: true,
      transparent: true,
      opacity: 0.3,
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
      opacity: 0.25,
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
      opacity: 0.3,
    })
    const icosahedron = new THREE.Mesh(icosahedronGeometry, icosahedronMaterial)
    icosahedron.position.y = 15
    scene.add(icosahedron)

    // Particle field
    const particlesGeometry = new THREE.BufferGeometry()
    const particleCount = 1000
    const positions = new Float32Array(particleCount * 3)
    const colors = new Float32Array(particleCount * 3)

    for (let i = 0; i < particleCount * 3; i += 3) {
      // Random positions in a large sphere
      positions[i] = (Math.random() - 0.5) * 100
      positions[i + 1] = (Math.random() - 0.5) * 100
      positions[i + 2] = (Math.random() - 0.5) * 100

      // Random colors between purple and blue
      const colorChoice = Math.random()
      if (colorChoice > 0.66) {
        colors[i] = purpleColor.r
        colors[i + 1] = purpleColor.g
        colors[i + 2] = purpleColor.b
      } else if (colorChoice > 0.33) {
        colors[i] = blueColor.r
        colors[i + 1] = blueColor.g
        colors[i + 2] = blueColor.b
      } else {
        colors[i] = cyanColor.r
        colors[i + 1] = cyanColor.g
        colors[i + 2] = cyanColor.b
      }
    }

    particlesGeometry.setAttribute(
      'position',
      new THREE.BufferAttribute(positions, 3),
    )
    particlesGeometry.setAttribute(
      'color',
      new THREE.BufferAttribute(colors, 3),
    )

    const particlesMaterial = new THREE.PointsMaterial({
      size: 0.15,
      vertexColors: true,
      transparent: true,
      opacity: 0.6,
      blending: THREE.AdditiveBlending,
    })

    const particles = new THREE.Points(particlesGeometry, particlesMaterial)
    scene.add(particles)

    // Grid helper for retro look
    const gridHelper = new THREE.GridHelper(100, 50, purpleColor, blueColor)
    gridHelper.material.transparent = true
    gridHelper.material.opacity = 0.1
    gridHelper.position.y = -20
    scene.add(gridHelper)

    // Animation
    let animationFrameId: number

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate)

      // Rotate geometries
      pyramid.rotation.x += 0.003
      pyramid.rotation.y += 0.005

      cube.rotation.x += 0.002
      cube.rotation.y += 0.003
      cube.rotation.z += 0.001

      icosahedron.rotation.x += 0.004
      icosahedron.rotation.y += 0.002

      // Rotate particle field slowly
      particles.rotation.y += 0.0005

      renderer.render(scene, camera)
    }

    animate()

    // Handle resize
    const handleResize = () => {
      camera.aspect = window.innerWidth / window.innerHeight
      camera.updateProjectionMatrix()
      renderer.setSize(window.innerWidth, window.innerHeight)
    }

    window.addEventListener('resize', handleResize)

    // Cleanup
    return () => {
      window.removeEventListener('resize', handleResize)
      cancelAnimationFrame(animationFrameId)
      renderer.dispose()
      pyramidGeometry.dispose()
      pyramidMaterial.dispose()
      cubeGeometry.dispose()
      cubeMaterial.dispose()
      icosahedronGeometry.dispose()
      icosahedronMaterial.dispose()
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
    <canvas
      ref={canvasRef}
      className="fixed inset-0 h-full w-full"
      style={{ zIndex: 0 }}
    />
  )
}
