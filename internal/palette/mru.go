package palette

// MRUCap is the largest size the persisted MRU ring grows to. The
// palette pulls these to the top whenever the user opens it.
const MRUCap = 10

// MRUBump returns a new MRU list with id moved to the front. The
// returned slice always has length <= MRUCap. Pure — caller persists.
func MRUBump(mru []uint16, id uint16) []uint16 {
	out := make([]uint16, 0, MRUCap)
	out = append(out, id)
	for _, x := range mru {
		if x == id {
			continue
		}
		out = append(out, x)
		if len(out) >= MRUCap {
			break
		}
	}
	return out
}
