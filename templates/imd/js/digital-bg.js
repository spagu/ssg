/**
 * Digital Network - Interactive & Visible
 * Reactive to Mouse
 */
(function () {
    function init() {
        let canvas = document.getElementById('digital-bg');
        if (!canvas) {
            canvas = document.createElement('canvas');
            canvas.id = 'digital-bg';
            document.body.prepend(canvas);
        }

        // Canvas fixes
        Object.assign(canvas.style, {
            position: 'fixed',
            top: '0',
            left: '0',
            width: '100%',
            height: '100%',
            zIndex: '-1', // Behind content
            background: '#ffffff' // White base
        });

        const ctx = canvas.getContext('2d');
        let width, height;
        let mouse = { x: null, y: null, radius: 200 };

        window.addEventListener('mousemove', function (event) {
            mouse.x = event.x;
            mouse.y = event.y;
        });

        function resize() {
            width = canvas.width = window.innerWidth;
            height = canvas.height = window.innerHeight;
        }
        window.addEventListener('resize', resize);
        resize();

        class Particle {
            constructor() {
                this.x = Math.random() * width;
                this.y = Math.random() * height;
                this.vx = (Math.random() - 0.5) * 1.5; // Faster
                this.vy = (Math.random() - 0.5) * 1.5;
                this.size = Math.random() * 3 + 1;
                this.baseX = this.x; // Remember original pos if we add return logic
                this.baseY = this.y;
                // Dark slate color
                this.color = '#334155';
            }

            update() {
                // Move
                this.x += this.vx;
                this.y += this.vy;

                // Bounce
                if (this.x < 0 || this.x > width) this.vx *= -1;
                if (this.y < 0 || this.y > height) this.vy *= -1;

                // Interaction with mouse
                if (mouse.x != null) {
                    let dx = mouse.x - this.x;
                    let dy = mouse.y - this.y;
                    let distance = Math.sqrt(dx * dx + dy * dy);

                    if (distance < mouse.radius) {
                        const forceDirectionX = dx / distance;
                        const forceDirectionY = dy / distance;
                        const maxDistance = mouse.radius;
                        const force = (maxDistance - distance) / maxDistance;
                        const directionX = forceDirectionX * force * 3; // Push strength
                        const directionY = forceDirectionY * force * 3;

                        // Attract (negative) or Repel (positive)? Let's attract (connect)
                        // Actually, connecting lines is better visual than pushing dots
                    }
                }
            }

            draw() {
                ctx.beginPath();
                ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
                ctx.fillStyle = this.color;
                ctx.fill();
            }
        }

        let particles = [];
        const particleCount = (width * height) / 9000; // Density
        for (let i = 0; i < particleCount; i++) {
            particles.push(new Particle());
        }

        function connect() {
            for (let a = 0; a < particles.length; a++) {
                for (let b = a; b < particles.length; b++) {
                    let distance = ((particles[a].x - particles[b].x) * (particles[a].x - particles[b].x)) +
                        ((particles[a].y - particles[b].y) * (particles[a].y - particles[b].y));

                    if (distance < (canvas.width / 7) * (canvas.height / 7)) {
                        let opacity = 1 - (distance / 20000);
                        if (opacity > 0) {
                            ctx.strokeStyle = 'rgba(71, 85, 105,' + opacity + ')'; // Slate 600
                            ctx.lineWidth = 1;
                            ctx.beginPath();
                            ctx.moveTo(particles[a].x, particles[a].y);
                            ctx.lineTo(particles[b].x, particles[b].y);
                            ctx.stroke();
                        }
                    }
                }

                // Connect to mouse
                if (mouse.x != null) {
                    let dx = mouse.x - particles[a].x;
                    let dy = mouse.y - particles[a].y;
                    let distance = (dx * dx + dy * dy);
                    if (distance < 30000) {
                        ctx.strokeStyle = 'rgba(0, 102, 204, 0.4)'; // Blue lines to mouse
                        ctx.lineWidth = 1.5;
                        ctx.beginPath();
                        ctx.moveTo(particles[a].x, particles[a].y);
                        ctx.lineTo(mouse.x, mouse.y);
                        ctx.stroke();
                    }
                }
            }
        }

        function animate() {
            ctx.clearRect(0, 0, width, height);
            for (let i = 0; i < particles.length; i++) {
                particles[i].update();
                particles[i].draw();
            }
            connect();
            requestAnimationFrame(animate);
        }

        animate();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
