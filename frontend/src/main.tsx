import React from 'react'
import {createRoot} from 'react-dom/client'
import {Hide} from '../wailsjs/runtime/runtime'
import './style.css'
import App from './App'

const container = document.getElementById('root')

window.addEventListener('keydown', (event) => {
    if (!event.metaKey || event.key.toLowerCase() !== 'w') {
        return
    }
    event.preventDefault()
    event.stopPropagation()
    Hide()
})

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
)
