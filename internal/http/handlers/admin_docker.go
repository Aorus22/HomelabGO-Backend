package handlers

import (
	"net/http"
	"sort"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/models"
)

type AdminDockerHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

func NewAdminDockerHandler(db *gorm.DB, docker *docker.Client) *AdminDockerHandler {
	return &AdminDockerHandler{db: db, docker: docker}
}

// Managed resource detection helper
func (h *AdminDockerHandler) getManagedProjects() (map[string]bool, error) {
	var deployments []models.Deployment
	if err := h.db.Find(&deployments).Error; err != nil {
		return nil, err
	}

	managed := make(map[string]bool)
	for _, d := range deployments {
		managed[d.ProjectName] = true
	}
	return managed, nil
}

// ListContainers lists all containers with managed badge
func (h *AdminDockerHandler) ListContainers(c *gin.Context) {
	api := h.docker.GetAPI()
	containers, err := api.ContainerList(c.Request.Context(), container.ListOptions{All: true})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list containers: " + err.Error()})
		return
	}

	managedProjects, _ := h.getManagedProjects()

	type ContainerResult struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Image       string            `json:"image"`
		State       string            `json:"state"`
		Status      string            `json:"status"`
		Created     int64             `json:"created"`
		ProjectName string            `json:"project_name"`
		IsManaged   bool              `json:"is_managed"`
		Labels      map[string]string `json:"labels"`
	}

	results := make([]ContainerResult, 0, len(containers))
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
			// remove leading slash
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		projectName := ctr.Labels["com.docker.compose.project"]
		isManaged := false
		if managedProjects != nil && projectName != "" {
			isManaged = managedProjects[projectName]
		}

		results = append(results, ContainerResult{
			ID:          ctr.ID[:12],
			Name:        name,
			Image:       ctr.Image,
			State:       ctr.State,
			Status:      ctr.Status,
			Created:     ctr.Created,
			ProjectName: projectName,
			IsManaged:   isManaged,
			Labels:      ctr.Labels,
		})
	}

	// Sort by managed first, then name
	sort.Slice(results, func(i, j int) bool {
		if results[i].IsManaged != results[j].IsManaged {
			return results[i].IsManaged
		}
		return results[i].Name < results[j].Name
	})

	c.JSON(http.StatusOK, results)
}

// ListImages lists all images
func (h *AdminDockerHandler) ListImages(c *gin.Context) {
	api := h.docker.GetAPI()
	images, err := api.ImageList(c.Request.Context(), image.ListOptions{All: false})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list images: " + err.Error()})
		return
	}

	type ImageResult struct {
		ID        string   `json:"id"`
		Tags      []string `json:"tags"`
		Size      int64    `json:"size"`
		Created   int64    `json:"created"`
		IsManaged bool     `json:"is_managed"` // Harder to determine for images, maybe if used by managed container?
	}

	results := make([]ImageResult, 0, len(images))
	for _, img := range images {
		// Cleanup ID (sha256:...)
		id := img.ID
		if len(id) > 19 {
			id = id[7:19]
		}

		results = append(results, ImageResult{
			ID:        id,
			Tags:      img.RepoTags,
			Size:      img.Size,
			Created:   img.Created,
			IsManaged: false, // TODO: cross reference with containers?
		})
	}

	c.JSON(http.StatusOK, results)
}

// ListNetworks lists all networks
func (h *AdminDockerHandler) ListNetworks(c *gin.Context) {
	api := h.docker.GetAPI()
	networks, err := api.NetworkList(c.Request.Context(), network.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list networks: " + err.Error()})
		return
	}

	managedProjects, _ := h.getManagedProjects()

	type NetworkResult struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Driver      string `json:"driver"`
		Scope       string `json:"scope"`
		Created     string `json:"created"` // API returns Time
		IsManaged   bool   `json:"is_managed"`
		ProjectName string `json:"project_name"`
	}

	results := make([]NetworkResult, 0, len(networks))
	for _, net := range networks {
		projectName := net.Labels["com.docker.compose.project"]
		isManaged := false
		if managedProjects != nil && projectName != "" {
			isManaged = managedProjects[projectName]
		}

		results = append(results, NetworkResult{
			ID:          net.ID[:12],
			Name:        net.Name,
			Driver:      net.Driver,
			Scope:       net.Scope,
			Created:     net.Created.String(),
			ProjectName: projectName,
			IsManaged:   isManaged,
		})
	}

	c.JSON(http.StatusOK, results)
}

// ListVolumes lists all volumes
func (h *AdminDockerHandler) ListVolumes(c *gin.Context) {
	api := h.docker.GetAPI()
	// VolumeList returns VolumeListOKBody
	volResponse, err := api.VolumeList(c.Request.Context(), volume.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list volumes: " + err.Error()})
		return
	}

	managedProjects, _ := h.getManagedProjects()

	type VolumeResult struct {
		Name        string `json:"name"`
		Driver      string `json:"driver"`
		Mountpoint  string `json:"mountpoint"`
		Created     string `json:"created"`
		IsManaged   bool   `json:"is_managed"`
		ProjectName string `json:"project_name"`
	}

	results := make([]VolumeResult, 0, len(volResponse.Volumes))
	for _, vol := range volResponse.Volumes {
		projectName := vol.Labels["com.docker.compose.project"]
		isManaged := false
		if managedProjects != nil && projectName != "" {
			isManaged = managedProjects[projectName]
		}

		created := ""
		// CreatedAt is string in Volume struct
		if vol.CreatedAt != "" {
			created = vol.CreatedAt
		}

		results = append(results, VolumeResult{
			Name:        vol.Name,
			Driver:      vol.Driver,
			Mountpoint:  vol.Mountpoint,
			Created:     created,
			ProjectName: projectName,
			IsManaged:   isManaged,
		})
	}

	c.JSON(http.StatusOK, results)
}
