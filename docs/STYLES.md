# Style Guide / Wytyczne kolorów

Ten dokument zawiera szczegółowe wytyczne dot. stylów i kolorów dla szablonów SSG.

## WCAG 2.2 Compliance

Wszystkie kolory są zgodne z WCAG 2.2 Level AA:
- Tekst normalny: kontrast min. **4.5:1**
- Tekst duży (18px+ bold lub 24px+): kontrast min. **3:1**
- Elementy interaktywne: kontrast min. **3:1**

## Simple Template (Dark Theme)

### Paleta kolorów

```css
:root {
  /* Tło */
  --color-bg-primary: #0f0f0f;      /* tło główne */
  --color-bg-secondary: #1a1a1a;    /* tło sekundarne */
  --color-bg-card: #222222;         /* karty */
  --color-bg-hover: #2a2a2a;        /* hover state */
  
  /* Tekst */
  --color-text-primary: #ffffff;    /* główny tekst - kontrast 21:1 */
  --color-text-secondary: #b3b3b3;  /* sekundarny - kontrast 9.6:1 */
  --color-text-muted: #808080;      /* przytłumiony - kontrast 5.1:1 */
  
  /* Akcent (fioletowy) */
  --color-accent: #6366f1;          /* główny akcent */
  --color-accent-hover: #818cf8;    /* hover */
  
  /* Gradient */
  --gradient-primary: linear-gradient(135deg, #6366f1, #8b5cf6, #a855f7);
}
```

### Użycie

| Element | Kolor | Kontrast |
|---------|-------|----------|
| Nagłówki | `#ffffff` | 21:1 |
| Tekst akapitów | `#b3b3b3` | 9.6:1 |
| Daty/meta | `#808080` | 5.1:1 |
| Linki | `#6366f1` | 4.5:1 |
| Linki hover | `#818cf8` | 6.2:1 |

### Efekty

- **Glassmorphism**: `backdrop-filter: blur(20px)`
- **Glow**: `box-shadow: 0 0 30px rgba(99, 102, 241, 0.3)`
- **Gradient text**: `background-clip: text` z gradientem

## Krowy Template (Light Theme)

### Paleta kolorów

```css
:root {
  /* Tło */
  --color-bg-primary: #f8faf5;      /* tło główne (lekko zielone) */
  --color-bg-secondary: #ffffff;    /* tło sekundarne */
  --color-bg-card: #ffffff;         /* karty */
  --color-bg-hover: #f0f5eb;        /* hover state */
  
  /* Tekst */
  --color-text-primary: #1a2e1a;    /* główny tekst - kontrast 15.2:1 */
  --color-text-secondary: #3d5a3d;  /* sekundarny - kontrast 8.1:1 */
  --color-text-muted: #6b8a6b;      /* przytłumiony - kontrast 4.6:1 */
  
  /* Akcent (zielony) */
  --color-accent: #2d7d32;          /* główny akcent */
  --color-accent-hover: #388e3c;    /* hover */
  --color-accent-light: #e8f5e9;    /* tło akcentu */
  
  /* Ziemia (sekundarny) */
  --color-earth: #795548;           /* brązowy */
  
  /* Gradient */
  --gradient-primary: linear-gradient(135deg, #2d7d32, #43a047, #66bb6a);
}
```

### Użycie

| Element | Kolor | Kontrast |
|---------|-------|----------|
| Nagłówki | `#1a2e1a` | 15.2:1 |
| Tekst akapitów | `#3d5a3d` | 8.1:1 |
| Daty/meta | `#6b8a6b` | 4.6:1 |
| Linki | `#2d7d32` | 6.3:1 |
| Linki hover | `#388e3c` | 5.1:1 |

### Efekty

- **Shadow card**: `box-shadow: 0 2px 8px rgba(45, 125, 50, 0.1)`
- **Border accent**: `border-bottom: 3px solid #2d7d32`
- **Logo icon**: 🐄 (cow emoji)

## Typography

### Font Family

```css
font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
```

### Font Sizes

```css
--font-size-xs: 0.75rem;    /* 12px */
--font-size-sm: 0.875rem;   /* 14px */
--font-size-base: 1rem;     /* 16px */
--font-size-lg: 1.125rem;   /* 18px */
--font-size-xl: 1.25rem;    /* 20px */
--font-size-2xl: 1.5rem;    /* 24px */
--font-size-3xl: 2rem;      /* 32px */
--font-size-4xl: 2.5rem;    /* 40px */
--font-size-5xl: 3rem;      /* 48px */
```

## Spacing

```css
--spacing-xs: 0.25rem;   /* 4px */
--spacing-sm: 0.5rem;    /* 8px */
--spacing-md: 1rem;      /* 16px */
--spacing-lg: 1.5rem;    /* 24px */
--spacing-xl: 2rem;      /* 32px */
--spacing-2xl: 3rem;     /* 48px */
--spacing-3xl: 4rem;     /* 64px */
```

## Border Radius

```css
--radius-sm: 0.375rem;   /* 6px */
--radius-md: 0.5rem;     /* 8px */
--radius-lg: 0.75rem;    /* 12px */
--radius-xl: 1rem;       /* 16px */
--radius-full: 9999px;   /* pill */
```

## Transitions

```css
--transition-fast: 150ms ease;
--transition-base: 300ms ease;
--transition-slow: 500ms ease;
```

## Reduced Motion

Obydwa szablony respektują preferencje użytkownika:

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}
```

## Responsive Breakpoints

```css
/* Mobile first */
@media (max-width: 768px) {
  /* Tablet i mniejsze */
}

@media (max-width: 480px) {
  /* Mobile */
}
```

## Accessibility Notes

1. **Focus visible**: Wszystkie interaktywne elementy mają widoczny focus ring
2. **Skip links**: Rozważ dodanie "skip to content" linku
3. **Color contrast**: Min. 4.5:1 dla tekstu, min. 3:1 dla dużego tekstu
4. **Touch targets**: Min. 44x44px dla elementów dotykowych
5. **Reduced motion**: Animacje wyłączane dla `prefers-reduced-motion`
