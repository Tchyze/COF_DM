try { const t = localStorage.getItem("cof-theme"); if (t) document.documentElement.dataset.theme = t; } catch (e) { /* stockage indisponible */ }
