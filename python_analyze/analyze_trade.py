import matplotlib.pyplot as plt
import numpy as np
import re
from datetime import datetime

# ============================================================================
# 📥 ДАННЫЕ: Извлечены из логов (16:45 - 06:15 MSK)
# Формат: (symbol, direction, entry_score, pnl, exit_type)
# ============================================================================
trades = [
    # MODEL DECAY TP (зелёные — обычно прибыльные)
    ("ETH", 1, 0.0585, 0.40, "Model Decay"),
    ("ETH", 1, 0.0807, 0.32, "Model Decay"),
    ("BNB", 1, 0.0951, 0.40, "Model Decay"),
    ("ATOM", 1, 0.2085, 0.76, "Model Decay"),
    ("LINK", -1, 0.0481, 0.33, "Model Decay"),
    ("TIA", -1, 0.0543, 0.51, "Model Decay"),
    ("APT", -1, 0.0451, 0.57, "Model Decay"),
    ("XRP", -1, 0.0538, 0.59, "Model Decay"),
    ("XRP", -1, 0.0545, 0.54, "Model Decay"),
    ("LINK", -1, 0.0589, 0.33, "Model Decay"),
    ("LINK", -1, 0.0710, 0.68, "Model Decay"),
    ("ETH", 1, 0.0572, 0.32, "Model Decay"),
    ("TIA", -1, 0.0638, 0.38, "Model Decay"),
    ("TIA", -1, 0.0625, 0.35, "Model Decay"),
    ("XRP", -1, 0.0793, 0.54, "Model Decay"),
    
    # SUBFRAME EXIT (красные — обычно убыточные)
    ("NEAR", 1, 0.2693, -0.43, "SubFrame"),
    ("BTC", 1, 0.0413, -0.81, "SubFrame"),
    ("BNB", 1, 0.0763, -0.91, "SubFrame"),
    ("NEAR", 1, 0.2168, -1.34, "SubFrame"),
    ("ATOM", 1, 0.2148, -0.57, "SubFrame"),
    ("HBAR", -1, 0.0502, -0.41, "SubFrame"),
    ("ARB", -1, 0.0503, -0.71, "SubFrame"),
    ("ETH", 1, 0.1016, -0.46, "SubFrame"),
    ("NEAR", 1, 0.1948, -1.34, "SubFrame"),
    ("HBAR", -1, 0.0525, -0.41, "SubFrame"),
    ("ARB", -1, 0.0581, -0.71, "SubFrame"),
    ("DOT", -1, 0.0456, -0.64, "SubFrame"),
    ("XRP", -1, 0.0527, -0.57, "SubFrame"),
    ("TIA", -1, 0.0788, -0.46, "SubFrame"),
    ("DOT", -1, 0.0456, -0.64, "SubFrame"),
    ("ARB", -1, 0.0581, -0.71, "SubFrame"),
    ("TIA", -1, 0.0625, -0.46, "SubFrame"),
    
    # ADAPTIVE EXIT (оранжевые — смешанные)
    ("BNB", 1, 0.0951, 0.16, "Adaptive"),
    ("AVAX", 1, 0.0555, 0.06, "Adaptive"),
    ("ETH", 1, 0.0718, 0.10, "Adaptive"),
    ("BNB", 1, 0.0595, -0.32, "Adaptive"),
    ("ATOM", 1, 0.1759, -0.01, "Adaptive"),
    ("ATOM", 1, 0.1439, -0.10, "Adaptive"),
    
    # SIGNAL EXIT (серые — около нуля)
    ("ATOM", 1, 0.2224, 0.00, "Signal"),
    ("DOGE", -1, 0.0564, 0.00, "Signal"),
    ("DOGE", -1, 0.0493, 0.00, "Signal"),
]

# ============================================================================
# 🎨 Визуализация
# ============================================================================
plt.figure(figsize=(14, 8))

# Цвета по типу выхода
colors = {
    "Model Decay": "#2ecc71",    # зелёный
    "SubFrame": "#e74c3c",        # красный
    "Adaptive": "#f39c12",        # оранжевый
    "Signal": "#95a5a6"           # серый
}

# Группируем данные
for exit_type in colors:
    subset = [t for t in trades if t[4] == exit_type]
    if not subset:
        continue
    scores = [t[2] for t in subset]
    pnls = [t[3] for t in subset]
    plt.scatter(scores, pnls, c=colors[exit_type], label=exit_type, 
                alpha=0.7, s=80, edgecolors='white', linewidth=0.5)

# Линии-подсказки
plt.axhline(y=0, color='black', linestyle='--', linewidth=0.5, alpha=0.3)
plt.axvline(x=0.040, color='blue', linestyle=':', linewidth=0.5, alpha=0.5, label='Long threshold (0.040)')
plt.axvline(x=0.045, color='red', linestyle=':', linewidth=0.5, alpha=0.5, label='Short threshold (0.045)')

# Линия тренда для Model Decay (самый важный тип)
model_decay = [t for t in trades if t[4] == "Model Decay"]
if len(model_decay) > 2:
    x = np.array([t[2] for t in model_decay])
    y = np.array([t[3] for t in model_decay])
    z = np.polyfit(x, y, 1)
    p = np.poly1d(z)
    plt.plot(x, p(x), "g--", alpha=0.5, label=f'Model Decay trend: y={z[0]:.2f}x+{z[1]:.2f}')

# Оформление
plt.xlabel('Entry Score (сила сигнала при входе)', fontsize=11)
plt.ylabel('PnL, %', fontsize=11)
plt.title('Markov Edge: Entry Score vs PnL\n(16:45–06:15 MSK, 36 сделок)', fontsize=13, fontweight='bold')
plt.legend(loc='upper right', fontsize=9)
plt.grid(True, alpha=0.2, linestyle=':')
plt.xlim(0.03, 0.28)
plt.ylim(-1.5, 1.0)

# Аннотации для ключевых точек
for t in trades:
    if abs(t[3]) > 0.7 or t[2] > 0.20:  # выделяем экстремумы
        plt.annotate(f"{t[0]}", (t[2], t[3]), 
                    textcoords="offset points", xytext=(5,5), 
                    fontsize=7, alpha=0.7)

plt.tight_layout()
plt.savefig('markov_score_vs_pnl.png', dpi=150, bbox_inches='tight')
plt.show()

# ============================================================================
# 📈 Статистика по группам
# ============================================================================
print("\n" + "="*60)
print("📊 СТАТИСТИКА ПО ГРУППАМ ВЫХОДОВ")
print("="*60)
for exit_type in colors:
    subset = [t for t in trades if t[4] == exit_type]
    if not subset:
        continue
    scores = [t[2] for t in subset]
    pnls = [t[3] for t in subset]
    print(f"\n{exit_type}:")
    print(f"  Кол-во сделок: {len(subset)}")
    print(f"  Ср. Score: {np.mean(scores):.4f} (min: {min(scores):.4f}, max: {max(scores):.4f})")
    print(f"  Ср. PnL: {np.mean(pnls):+.2f}%")
    print(f"  Win Rate: {sum(1 for p in pnls if p > 0)/len(pnls)*100:.1f}%")
    print(f"  Best: {max(pnls):+.2f}% | Worst: {min(pnls):+.2f}%")