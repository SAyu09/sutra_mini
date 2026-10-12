-- Idempotent demo seed data for Sutra Mini

-- 1. Insert 5 SKUs
INSERT INTO skus (code, name, image_url)
VALUES
  ('SKU-KURTA-001', 'Chanderi Silk Straight Kurta', 'https://images.unsplash.com/photo-1610030469983-98e550d6193c?auto=format&fit=crop&w=400&q=80'),
  ('SKU-ANARKALI-002', 'Floral Anarkali Suit Set', 'https://images.unsplash.com/photo-1583391733956-3750e0ff4e8b?auto=format&fit=crop&w=400&q=80'),
  ('SKU-LEHENGA-003', 'Raw Silk Zardozi Lehenga', 'https://images.unsplash.com/photo-1617627143750-d86bc21e42bb?auto=format&fit=crop&w=400&q=80'),
  ('SKU-DUPATTA-004', 'Banarasi Brocade Dupatta', 'https://images.unsplash.com/photo-1609357605129-26f69add5d6e?auto=format&fit=crop&w=400&q=80'),
  ('SKU-SALWAR-005', 'Cotton Satin Salwar Kameez', 'https://images.unsplash.com/photo-1596783074918-c84cb06531ca?auto=format&fit=crop&w=400&q=80')
ON CONFLICT (code) DO NOTHING;

-- 2. Insert 2 Job-Workers (parties)
INSERT INTO parties (name, phone)
SELECT 'Sharma Embroidery Works', '+91 98101 23456'
WHERE NOT EXISTS (
  SELECT 1 FROM parties WHERE name = 'Sharma Embroidery Works'
);

INSERT INTO parties (name, phone)
SELECT 'Verma Stitching Studio', '+91 98112 34567'
WHERE NOT EXISTS (
  SELECT 1 FROM parties WHERE name = 'Verma Stitching Studio'
);

-- 3. Insert 4 OPEN Jobwork Issues
INSERT INTO jobwork_issues (party_id, sku_id, qty, received_qty, rate_paise, issued_on)
SELECT p.id, s.id, 50, 0, 4500, '2026-10-01'
FROM parties p, skus s
WHERE p.name = 'Sharma Embroidery Works' AND s.code = 'SKU-KURTA-001'
  AND NOT EXISTS (
    SELECT 1 FROM jobwork_issues j
    WHERE j.party_id = p.id AND j.sku_id = s.id AND j.issued_on = '2026-10-01'
  );

INSERT INTO jobwork_issues (party_id, sku_id, qty, received_qty, rate_paise, issued_on)
SELECT p.id, s.id, 25, 0, 15000, '2026-10-03'
FROM parties p, skus s
WHERE p.name = 'Sharma Embroidery Works' AND s.code = 'SKU-LEHENGA-003'
  AND NOT EXISTS (
    SELECT 1 FROM jobwork_issues j
    WHERE j.party_id = p.id AND j.sku_id = s.id AND j.issued_on = '2026-10-03'
  );

INSERT INTO jobwork_issues (party_id, sku_id, qty, received_qty, rate_paise, issued_on)
SELECT p.id, s.id, 40, 0, 12000, '2026-10-05'
FROM parties p, skus s
WHERE p.name = 'Verma Stitching Studio' AND s.code = 'SKU-ANARKALI-002'
  AND NOT EXISTS (
    SELECT 1 FROM jobwork_issues j
    WHERE j.party_id = p.id AND j.sku_id = s.id AND j.issued_on = '2026-10-05'
  );

INSERT INTO jobwork_issues (party_id, sku_id, qty, received_qty, rate_paise, issued_on)
SELECT p.id, s.id, 80, 0, 8000, '2026-10-08'
FROM parties p, skus s
WHERE p.name = 'Verma Stitching Studio' AND s.code = 'SKU-SALWAR-005'
  AND NOT EXISTS (
    SELECT 1 FROM jobwork_issues j
    WHERE j.party_id = p.id AND j.sku_id = s.id AND j.issued_on = '2026-10-08'
  );
