export function ConnectionVisual() {
  return (
    <svg className="connection-visual" viewBox="0 0 610 210" role="img" aria-label="电脑与手机通过蓝牙安全连接">
      <defs>
        <linearGradient id="deviceSurface" x1="0" y1="0" x2="1" y2="1">
          <stop stopColor="#ffffff" />
          <stop offset="1" stopColor="#edf5ff" />
        </linearGradient>
        <linearGradient id="shieldBlue" x1="0" y1="0" x2="0" y2="1">
          <stop stopColor="#3d8cff" />
          <stop offset="1" stopColor="#1262e8" />
        </linearGradient>
        <filter id="deviceShadow" x="-30%" y="-30%" width="160%" height="180%">
          <feDropShadow dx="0" dy="10" stdDeviation="10" floodColor="#3d69a8" floodOpacity=".15" />
        </filter>
      </defs>
      <g className="connection-wave" fill="none" strokeLinecap="round">
        <path d="M196 66c43 0 53-29 94-29s51 29 92 29 53-29 94-29 51 29 91 29" stroke="#1f72ff" strokeWidth="2.5" strokeDasharray="5 9" />
        <path d="M196 99c43 0 53-22 94-22s51 22 92 22 53-22 94-22 51 22 91 22" stroke="#9dc4ff" strokeWidth="2" strokeDasharray="4 9" />
        <path d="M196 132c43 0 53-29 94-29s51 29 92 29 53-29 94-29 51 29 91 29" stroke="#2b7bff" strokeWidth="2.3" strokeDasharray="5 9" />
      </g>
      <g filter="url(#deviceShadow)">
        <rect x="12" y="20" width="198" height="165" rx="24" fill="url(#deviceSurface)" stroke="#c9ddf5" />
        <rect x="47" y="48" width="128" height="91" rx="6" fill="#fff" stroke="#2e75e8" strokeWidth="4" />
        <path d="M37 147h148l-9 10H46l-9-10Z" fill="#142861" />
        <path d="M111 65 132 74v18c0 15-9 25-21 30-12-5-21-15-21-30V74l21-9Z" fill="url(#shieldBlue)" />
        <path d="m111 75 9 9-7 7 7 7-9 9V75Zm0 16-7-7m7 7-7 7" fill="none" stroke="#fff" strokeWidth="2.3" strokeLinecap="round" strokeLinejoin="round" />
      </g>
      <g className="bluetooth-orbit">
        <circle cx="382" cy="99" r="22" fill="#2c7fff" />
        <path d="m382 85 9 9-7 6 7 6-9 9V85Zm0 15-7-6m7 6-7 6" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </g>
      <g filter="url(#deviceShadow)">
        <rect x="500" y="26" width="92" height="154" rx="22" fill="url(#deviceSurface)" stroke="#c9ddf5" />
        <rect x="520" y="45" width="52" height="112" rx="8" fill="#fafdff" stroke="#2977f5" strokeWidth="2" />
        <path d="m546 83 10 10-7 7 7 7-10 10V83Zm0 17-8-7m8 7-8 7" fill="none" stroke="#1f72ff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx="580" cy="159" r="18" fill="#12ac79" />
        <path d="m572 159 5 5 10-12" fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
      </g>
    </svg>
  );
}
