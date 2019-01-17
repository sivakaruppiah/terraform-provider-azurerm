package azurerm

import (
	"fmt"
	"log"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/services/mediaservices/mgmt/2018-07-01/media"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/response"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

/*
 Example terraform template:

	resource "azurerm_media_services" "ams" {

			name = "seushertestams2"
			location = "westus"
			resource_group_name = "seusher_dev"

			storage_account {
					id = "/subscriptions/7060bca0-7a3c-44bd-b54c-4bb1e9facfac/resourcegroups/seusher_dev/providers/Microsoft.Storage/storageAccounts/seusherams3"
					is_primary = true
			}

			storage_account {
					id = "/subscriptions/7060bca0-7a3c-44bd-b54c-4bb1e9facfac/resourcegroups/seusher_dev/providers/Microsoft.Storage/storageAccounts/seusheram2"
					is_primary = false
			}
	}

	output "rendered" {
	value = "${azurerm_media_services.ams.id}"
	}
*/

func resourceArmMediaServices() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmMediaServicesCreateUpdate,
		Read:   resourceArmMediaServicesRead,
		Update: resourceArmMediaServicesCreateUpdate,
		Delete: resourceArmMediaServicesDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		// TODO: Add validation after finding out the rules for AMS names
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringMatch(
					regexp.MustCompile("^[-a-z0-9]{3,50}$"),
					"Media Services Account name must be 3 - 50 characters long, contain only letters, numbers and hyphens.",
				),
			},

			"location": locationSchema(),

			"resource_group_name": resourceGroupNameSchema(),

			"tags": tagsSchema(),

			"storage_account": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},

						"is_primary": {
							Type:     schema.TypeBool,
							Required: true,
						},
					},
				},
			},
		},
	}
}

func resourceArmMediaServicesCreateUpdate(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*ArmClient).mediaServicesClient
	ctx := meta.(*ArmClient).StopContext

	accountName := d.Get("name").(string)
	location := azureRMNormalizeLocation(d.Get("location").(string))
	tags := d.Get("tags").(map[string]interface{})
	resourceGroup := d.Get("resource_group_name").(string)

	storageAccounts, err := expandAzureRmStorageAccounts(d)
	if err != nil {
		return err
	}

	parameters := media.Service{
		ServiceProperties: &media.ServiceProperties{
			StorageAccounts: &storageAccounts,
		},
		Location: utils.String(location),
		Tags:     expandTags(tags),
	}

	service, err := client.CreateOrUpdate(ctx, resourceGroup, accountName, parameters)
	if err != nil {
		return fmt.Errorf("Error creating Media Service Account: %+v", err)
	}

	d.SetId(*service.ID)

	return nil
}

func resourceArmMediaServicesRead(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*ArmClient).mediaServicesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return fmt.Errorf("Error parsing Azure Resource ID %q: %+v", d.Id(), err)
	}

	name := id.Path["mediaservices"]
	resourceGroup := id.ResourceGroup

	resp, err := client.Get(ctx, resourceGroup, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Error reading Media Services Account %q - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error reading Media Services Account: %+v", err)
	}

	d.Set("name", resp.Name)
	if location := resp.Location; location != nil {
		d.Set("location", azureRMNormalizeLocation(*location))
	}
	flattenAndSetTags(d, resp.Tags)

	return nil
}

func resourceArmMediaServicesDelete(d *schema.ResourceData, meta interface{}) error {

	client := meta.(*ArmClient).mediaServicesClient
	ctx := meta.(*ArmClient).StopContext

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return fmt.Errorf("Error parsing Azure Resource ID %q: %+v", d.Id(), err)
	}

	name := id.Path["mediaservices"]
	resourceGroup := id.ResourceGroup

	httpResponse, err := client.Delete(ctx, resourceGroup, name)
	if err != nil {
		if response.WasNotFound(httpResponse.Response) {
			return nil
		}
		return fmt.Errorf("Error issuing AzureRM delete request for Media Services Account '%s': %+v", name, err)
	}

	return nil
}

func expandAzureRmStorageAccounts(d *schema.ResourceData) ([]media.StorageAccount, error) {
	storageAccounts := d.Get("storage_account").(*schema.Set).List()
	rules := make([]media.StorageAccount, 0)

	// Only one storage account can be primary
	primaryAssigned := false

	for _, accountMapRaw := range storageAccounts {
		accountMap := accountMapRaw.(map[string]interface{})

		id := accountMap["id"].(string)

		storageType := media.Secondary
		if accountMap["is_primary"].(bool) {
			storageType = media.Primary

			// TODO: This function shouldn't process storage accounts and validate them. Move logic out appropriately.
			if primaryAssigned {
				return nil, fmt.Errorf("Error processing storage account '%s'. Another storage account is already assigned as is_primary = 'true'", id)
			}

			primaryAssigned = true
		}

		storageAccount := media.StorageAccount{
			ID:   utils.String(id),
			Type: storageType,
		}

		rules = append(rules, storageAccount)
	}

	return rules, nil
}
