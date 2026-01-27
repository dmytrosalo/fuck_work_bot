"""
Work Message Classifier (Light)
Lightweight classifier for Telegram bot on Fly.io

Size: 0.53 MB
Speed: <2ms
Accuracy: 99.08%

pip install scikit-learn joblib
"""

import joblib
import os
import numpy as np
from typing import Dict, List, Optional
from sklearn.base import BaseEstimator, TransformerMixin

WORK_KEYWORDS = [
    'keyo', 'кейо', 'pos', 'пос', 'nrf', 'нрф', 'девайс', 'device', 'біометр',
    'stripe', 'страйп', 'biopay', 'біопей', 'api', 'sdk', 'backend', 'бекенд',
    'frontend', 'фронтенд', 'андроїд', 'android', 'ios', 'застосунок', 'апка',
    'сканінг', 'енрол', 'транзакц', 'payment', 'платіж', 'деплой', 'deploy',
    'тікет', 'ticket', 'джира', 'jira', 'лінеар', 'linear', 'мітинг', 'meeting',
    'стендап', 'standup', 'дейлі', 'daily', 'спринт', 'sprint', 'реліз', 'release',
    'мердж', 'merge', 'код', 'code', 'баг', 'bug', 'фікс', 'fix',
    'рев\'ю', 'review', 'естімейт', 'дедлайн', 'deadline', 'пайплайн', 'ci/cd',
    'маріт', 'marit', 'ілір', 'ilir', 'делна', 'delna', 'насір', 'nassir',
    'руді', 'rudi', 'аршаан', 'даглас', 'сільвейн', 'silvain', 'тамара', 'конг',
    'валер', 'алек', 'нуно', 'азам', 'лід', 'lead',
    'tenderize', 'тендерайз', 'hexaon', 'масарі', 'masari',
    'команд', 'team', 'тім', 'проєкт', 'project', 'клієнт', 'client',
    'менеджер', 'manager', 'директор', 'director', 'cto',
    'зарплат', 'salary', 'рейз', 'відпустк', 'густо', 'gusto', 'діл', 'deel',
    'контракт', 'сервер', 'server', 'сокет', 'websocket', 'ендпоінт', 'флоу', 'flow',
    'імплемент', 'компіл', 'білд', 'build', 'xcode', 'gradle', 'слек', 'slack',
    'демо', 'demo',
]


class KeywordFeatures(BaseEstimator, TransformerMixin):
    """Transformer for keyword-based features"""

    def __init__(self, keywords=None):
        self.keywords = [kw.lower() for kw in (keywords or WORK_KEYWORDS)]

    def fit(self, X, y=None):
        return self

    def transform(self, X):
        features = []
        for text in X:
            text_lower = text.lower() if isinstance(text, str) else ""
            kw_count = sum(1 for kw in self.keywords if kw in text_lower)
            has_kw = 1 if kw_count > 0 else 0
            words = len(text_lower.split())
            density = kw_count / max(words, 1)
            char_count = len(text_lower)
            has_question = 1 if '?' in text_lower else 0
            features.append([kw_count, has_kw, density, char_count, words, has_question])
        return np.array(features)


# Register for pickle
import __main__
if not hasattr(__main__, 'KeywordFeatures'):
    __main__.KeywordFeatures = KeywordFeatures


class WorkClassifier:
    """Work message classifier"""

    def __init__(self, model_path: Optional[str] = None):
        if model_path is None:
            current_dir = os.path.dirname(os.path.abspath(__file__))
            model_path = os.path.join(current_dir, 'work_classifier_light.joblib')

        if not os.path.exists(model_path):
            raise FileNotFoundError(f"Model not found: {model_path}")

        self.model = joblib.load(model_path)

    def predict(self, text: str) -> Dict:
        """Classifies a message"""
        if not text or not text.strip():
            return {'label': 'personal', 'confidence': 1.0, 'is_work': False}

        pred = self.model.predict([text])[0]
        proba = self.model.predict_proba([text])[0]
        confidence = proba[1] if pred == 'work' else proba[0]

        return {
            'label': pred,
            'confidence': float(confidence),
            'is_work': pred == 'work'
        }

    def predict_batch(self, texts: List[str]) -> List[Dict]:
        """Classifies a list of messages"""
        return [self.predict(text) for text in texts]

    def is_work(self, text: str) -> bool:
        """Fast check"""
        return self.predict(text)['is_work']


# Singleton
_classifier = None

def get_classifier() -> WorkClassifier:
    global _classifier
    if _classifier is None:
        _classifier = WorkClassifier()
    return _classifier


def is_work_message(text: str) -> bool:
    return get_classifier().is_work(text)


def classify_message(text: str) -> Dict:
    return get_classifier().predict(text)


if __name__ == "__main__":
    import sys
    clf = WorkClassifier()

    if len(sys.argv) > 1:
        text = " ".join(sys.argv[1:])
        r = clf.predict(text)
        emoji = "💼" if r['is_work'] else "😎"
        print(f"{emoji} {r['label']} ({r['confidence']:.0%})")
    else:
        print("Work Classifier | 'q' to quit\n")
        while True:
            try:
                text = input("> ").strip()
                if text.lower() == 'q':
                    break
                if text:
                    r = clf.predict(text)
                    emoji = "💼" if r['is_work'] else "😎"
                    print(f"  {emoji} {r['label']} ({r['confidence']:.0%})\n")
            except KeyboardInterrupt:
                break
